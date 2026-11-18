package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"neuralnetwork/internal/engine"
	"neuralnetwork/internal/models"
	"neuralnetwork/internal/server"
	"neuralnetwork/internal/storage"
)

func main() {
	gatewayMode := flag.Bool("gateway", false, "Ejecuta únicamente el proxy inverso puente hacia el contenedor en network_mode: host")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *gatewayMode {
		listenPort := getEnv("NN_PORT", "8080")
		upstreamURL := os.Getenv("NN_UPSTREAM_URL")
		if err := server.RunGatewayProxy(ctx, ":"+listenPort, upstreamURL); err != nil {
			log.Fatalf("[gateway] error fatal: %v", err)
		}
		return
	}

	internalPort := getEnvInt("NN_INTERNAL_PORT", 18080)
	publicPort := getEnvInt("NN_PORT", 8080)
	listenPort := internalPort
	if strings.EqualFold(os.Getenv("NN_LISTEN_DIRECT"), "true") {
		listenPort = publicPort
	}

	dbPath := getEnv("NN_DB_PATH", "/data/neuralnetwork.db")
	staticDir := getEnv("NN_STATIC_DIR", "/app/public")
	iface := getEnv("NN_INTERFACE", "auto")
	defaultMode := getEnv("NN_DEFAULT_MODE", "live")
	if defaultMode != "live" && defaultMode != "simulation" {
		defaultMode = "live"
	}
	ttlSec := getEnvInt("NN_NODE_TTL_SEC", 120)
	promisc := !strings.EqualFold(os.Getenv("NN_PROMISCUOUS"), "false")

	store, err := storage.NewStore(dbPath)
	if err != nil {
		log.Fatalf("[storage] error inicializando SQLite: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("[storage] error cerrando SQLite: %v", err)
		}
	}()

	initialSettings := models.AppSettings{
		GroupExternal: false,
		NodeTTLSec:    ttlSec,
		BloomStrength: 1.15,
		CaptureMode:   defaultMode,
		Interface:     iface,
	}

	mgr, err := engine.NewManager(store, initialSettings, promisc, []int{internalPort, publicPort})
	if err != nil {
		log.Fatalf("[engine] error inicializando motor: %v", err)
	}

	srv := server.NewServer(mgr, staticDir)
	mgr.Start(ctx)
	defer mgr.Stop()

	httpServer := &http.Server{
		Addr:              ":" + strconv.Itoa(listenPort),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	snap := mgr.GetStateSnapshot()
	log.Printf("[neuralnetwork] Servidor escuchando en :%d (modo=%s, interfaz=%s, db=%s)",
		listenPort, snap.Stats.CaptureMode, snap.Stats.Interface, dbPath)

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("[server] error HTTP: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
