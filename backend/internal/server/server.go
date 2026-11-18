package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"neuralnetwork/internal/capture"
	"neuralnetwork/internal/engine"
	"neuralnetwork/internal/models"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 16384,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type wsClient struct {
	conn *websocket.Conn
	send chan []byte
}

// WSHub gestiona los clientes WebSocket conectados y transmite ticks en tiempo real.
type WSHub struct {
	mu      sync.RWMutex
	clients map[*wsClient]struct{}
}

func NewWSHub() *WSHub {
	return &WSHub{
		clients: make(map[*wsClient]struct{}),
	}
}

func (h *WSHub) Add(c *wsClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *WSHub) Remove(c *wsClient) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

func (h *WSHub) Broadcast(payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- payload:
		default:
			// Descartar frame para clientes lentos sin bloquear el motor en tiempo real.
		}
	}
}

// Server expone la API REST, WebSocket y archivos estáticos del frontend.
type Server struct {
	manager   *engine.Manager
	hub       *WSHub
	staticDir string
}

func NewServer(manager *engine.Manager, staticDir string) *Server {
	hub := NewWSHub()
	manager.SetOnTick(func(msg models.TickMessage) {
		data, err := json.Marshal(msg)
		if err != nil {
			return
		}
		hub.Broadcast(data)
	})
	return &Server{
		manager:   manager,
		hub:       hub,
		staticDir: staticDir,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/interfaces", s.handleInterfaces)
	mux.HandleFunc("/api/capture/mode", s.handleCaptureMode)
	mux.HandleFunc("/api/topology/clear", s.handleTopologyClear)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/nodes/", s.handleNodeByIP)
	mux.HandleFunc("/ws", s.handleWS)

	if s.staticDir != "" {
		if info, err := os.Stat(s.staticDir); err == nil && info.IsDir() {
			mux.Handle("/", spaHandler(s.staticDir))
		}
	}
	return withCORSHeaders(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.manager.GetStateSnapshot())
}

func (s *Server) handleInterfaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ifaces := s.manager.RefreshInterfaces()
	snap := s.manager.GetStateSnapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"interfaces":  ifaces,
		"captureMode": snap.Stats.CaptureMode,
		"active":      snap.Stats.Interface,
	})
}

func (s *Server) handleCaptureMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Mode      string `json:"mode"`
		Interface string `json:"interface"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if err := s.manager.SwitchCaptureMode(req.Mode, strings.TrimSpace(req.Interface)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.manager.GetStateSnapshot())
}

func (s *Server) handleTopologyClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.manager.ClearTopology()
	writeJSON(w, http.StatusOK, s.manager.GetStateSnapshot())
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.manager.GetStateSnapshot().Settings)
	case http.MethodPut:
		var req struct {
			GroupExternal *bool    `json:"groupExternal"`
			NodeTTLSec    *int     `json:"nodeTtlSec"`
			BloomStrength *float64 `json:"bloomStrength"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		updated, err := s.manager.UpdateSettings(req.GroupExternal, req.NodeTTLSec, req.BloomStrength)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, updated)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleNodeByIP(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimPrefix(r.URL.Path, "/api/nodes/")
	ip, err := url.PathUnescape(raw)
	if err != nil || ip == "" {
		writeError(w, http.StatusBadRequest, "IP de nodo inválida")
		return
	}
	if ip != "0.0.0.0/0" && net.ParseIP(ip) == nil {
		writeError(w, http.StatusBadRequest, "formato de IP inválido")
		return
	}

	switch r.Method {
	case http.MethodPut:
		var req models.NodeCustomization
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		req.IP = ip
		req.Alias = strings.TrimSpace(req.Alias)
		if len(req.Alias) > 80 {
			writeError(w, http.StatusBadRequest, "el alias no puede superar los 80 caracteres")
			return
		}
		if req.Category == "" {
			req.Category = "default"
		}
		if err := s.manager.SaveNodeCustomization(req); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, req)
	case http.MethodDelete:
		if err := s.manager.DeleteNodeCustomization(ip); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"deleted": ip})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &wsClient{
		conn: conn,
		send: make(chan []byte, 16),
	}
	s.hub.Add(client)

	if initData, err := json.Marshal(s.manager.GetInitMessage()); err == nil {
		client.send <- initData
	}

	go s.wsWritePump(client)
	s.wsReadPump(client)
}

func (s *Server) wsWritePump(c *wsClient) {
	pingTicker := time.NewTicker(25 * time.Second)
	defer func() {
		pingTicker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-pingTicker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *Server) wsReadPump(c *wsClient) {
	defer func() {
		s.hub.Remove(c)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			break
		}
	}
}

// RunGatewayProxy ejecuta un proxy inverso HTTP + WebSocket hacia el contenedor principal en network_mode: host.
func RunGatewayProxy(ctx context.Context, listenAddr, upstreamURL string) error {
	if upstreamURL == "" {
		gw := capture.DetectDefaultGateway()
		if gw == "" {
			gw = "127.0.0.1"
		}
		internalPort := os.Getenv("NN_INTERNAL_PORT")
		if internalPort == "" {
			internalPort = "18080"
		}
		upstreamURL = "http://" + net.JoinHostPort(gw, internalPort)
	}

	target, err := url.Parse(upstreamURL)
	if err != nil {
		return err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = 20 * time.Millisecond

	mux := http.NewServeMux()
	mux.HandleFunc("/gateway-health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":   "ok",
			"upstream": upstreamURL,
		})
	})
	mux.Handle("/", proxy)

	srv := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("[gateway] Proxy inverso activo en %s -> %s", listenAddr, upstreamURL)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func spaHandler(root string) http.Handler {
	fileServer := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if cleanPath == "." {
			cleanPath = "index.html"
		}
		fullPath := filepath.Join(root, cleanPath)
		info, err := os.Stat(fullPath)
		if err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		if errors.Is(err, fs.ErrNotExist) || (err == nil && info.IsDir()) {
			http.ServeFile(w, r, filepath.Join(root, "index.html"))
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func withCORSHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
