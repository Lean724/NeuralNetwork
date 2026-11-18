package engine

import (
	"context"
	"fmt"
	"math"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"neuralnetwork/internal/capture"
	"neuralnetwork/internal/models"
	"neuralnetwork/internal/storage"
)

const (
	externalGroupID = "0.0.0.0/0"
	tickInterval    = 100 * time.Millisecond
)

type internalNode struct {
	id           string
	ip           string
	mac          string
	vendor       string
	hostname     string
	role         string
	rxBps        float64
	txBps        float64
	rxPps        float64
	txPps        float64
	winRxBytes   uint64
	winTxBytes   uint64
	winRxPkts    uint64
	winTxPkts    uint64
	totalRxBytes uint64
	totalTxBytes uint64
	totalPackets uint64
	protocols    map[string]uint64
	ports        map[uint16]uint64
	peers        map[string]*peerAccum
	lastSeen     time.Time
	energy       float64
}

type peerAccum struct {
	ip       string
	bytes    uint64
	protocol string
}

type internalLink struct {
	id           string
	source       string
	target       string
	bps          float64
	pps          float64
	winBytes     uint64
	winPkts      uint64
	totalBytes   uint64
	totalPackets uint64
	protocols    map[string]uint64
	ports        map[uint16]uint64
	lastSeen     time.Time
}

type flowKey struct {
	src      string
	dst      string
	protocol string
}

type pulseAccum struct {
	packets uint64
	bytes   uint64
	ports   map[uint16]struct{}
	info    string
}

// Manager mantiene el estado en memoria de la topología y agrupa los pulsos cada 100 ms.
type Manager struct {
	mu           sync.RWMutex
	store        *storage.Store
	settings     models.AppSettings
	customs      map[string]models.NodeCustomization
	nodes        map[string]*internalNode
	links        map[string]*internalLink
	pulseWindow  map[flowKey]*pulseAccum
	dnsCache     map[string]string
	dnsInFlight  map[string]bool
	hostIPs      map[string]bool
	gatewayIP    string
	interfaces   []models.NetworkInterface
	captureErr   string
	packetsSeen  uint64
	promisc      bool
	excludedPort []int

	packetCh   chan models.PacketEvent
	sourceMu   sync.Mutex
	sourceStop context.CancelFunc
	rootCtx    context.Context
	rootCancel context.CancelFunc
	pulseSeq   atomic.Uint64

	onTick func(models.TickMessage)
}

// NewManager crea e inicializa el motor de topología y agregación adaptativa.
func NewManager(store *storage.Store, initialSettings models.AppSettings, promisc bool, excludedPorts []int) (*Manager, error) {
	customs := make(map[string]models.NodeCustomization)
	settings := initialSettings

	if store != nil {
		var err error
		customs, err = store.GetKnownNodes()
		if err != nil {
			return nil, fmt.Errorf("error cargando nodos conocidos: %w", err)
		}
		settings, err = store.GetSettings(initialSettings)
		if err != nil {
			return nil, fmt.Errorf("error cargando configuración: %w", err)
		}
	}

	ifaces, _ := capture.ListInterfaces()
	if settings.Interface == "" || settings.Interface == "auto" {
		settings.Interface = capture.SelectDefaultInterface("auto", ifaces)
	}
	if settings.NodeTTLSec < 10 {
		settings.NodeTTLSec = 120
	}
	if settings.CaptureMode == "" {
		settings.CaptureMode = "live"
	}

	hostIPs, gatewayIP := capture.DetectHostNetwork()

	m := &Manager{
		store:        store,
		settings:     settings,
		customs:      customs,
		nodes:        make(map[string]*internalNode),
		links:        make(map[string]*internalLink),
		pulseWindow:  make(map[flowKey]*pulseAccum),
		dnsCache:     make(map[string]string),
		dnsInFlight:  make(map[string]bool),
		hostIPs:      hostIPs,
		gatewayIP:    gatewayIP,
		interfaces:   ifaces,
		promisc:      promisc,
		excludedPort: excludedPorts,
		packetCh:     make(chan models.PacketEvent, 8192),
	}

	if settings.CaptureMode == "simulation" {
		m.applySimulatedMetadata()
	}

	return m, nil
}

// SetOnTick registra el callback que recibirá los mensajes TickMessage cada 100 ms.
func (m *Manager) SetOnTick(fn func(models.TickMessage)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onTick = fn
}

// Start inicia el bucle de procesamiento de paquetes, el ticker de 100 ms y la fuente activa.
func (m *Manager) Start(ctx context.Context) {
	m.rootCtx, m.rootCancel = context.WithCancel(ctx)
	go m.processLoop()
	m.startSource()
}

// Stop detiene las goroutines de captura y agregación.
func (m *Manager) Stop() {
	m.sourceMu.Lock()
	if m.sourceStop != nil {
		m.sourceStop()
		m.sourceStop = nil
	}
	m.sourceMu.Unlock()

	if m.rootCancel != nil {
		m.rootCancel()
	}
}

func (m *Manager) startSource() {
	m.sourceMu.Lock()
	defer m.sourceMu.Unlock()

	if m.sourceStop != nil {
		m.sourceStop()
		m.sourceStop = nil
	}

	m.mu.Lock()
	mode := m.settings.CaptureMode
	iface := m.settings.Interface
	m.captureErr = ""
	if mode == "simulation" {
		m.applySimulatedMetadata()
	} else {
		hostIPs, gw := capture.DetectHostNetwork()
		m.hostIPs = hostIPs
		m.gatewayIP = gw
	}
	m.mu.Unlock()

	if m.rootCtx == nil {
		return
	}

	srcCtx, cancel := context.WithCancel(m.rootCtx)
	m.sourceStop = cancel

	if mode == "simulation" {
		go capture.RunSimulation(srcCtx, m.packetCh)
		return
	}

	go func(targetIface string) {
		err := capture.RunLiveCapture(srcCtx, targetIface, m.promisc, m.excludedPort, m.packetCh)
		if err != nil && srcCtx.Err() == nil {
			m.mu.Lock()
			m.captureErr = err.Error()
			m.mu.Unlock()
		}
	}(iface)
}

func (m *Manager) applySimulatedMetadata() {
	hosts, gw, names := capture.SimulatedHostSet()
	m.hostIPs = hosts
	m.gatewayIP = gw
	for ip, name := range names {
		m.dnsCache[ip] = name
	}
}

func (m *Manager) processLoop() {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.rootCtx.Done():
			return
		case ev := <-m.packetCh:
			m.IngestPacket(ev)
		case <-ticker.C:
			// Drenar ráfaga acumulada en el canal antes de consolidar la ventana
			drain := true
			for drain {
				select {
				case ev := <-m.packetCh:
					m.IngestPacket(ev)
				default:
					drain = false
				}
			}
			msg := m.FlushWindow(time.Now())
			m.mu.RLock()
			cb := m.onTick
			m.mu.RUnlock()
			if cb != nil {
				cb(msg)
			}
		}
	}
}

// IngestPacket incorpora un evento de paquete en la topología y en la ventana de pulsos actual.
func (m *Manager) IngestPacket(ev models.PacketEvent) {
	if ev.SrcIP == "" || ev.DstIP == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := ev.Timestamp
	if now.IsZero() {
		now = time.Now()
	}

	// Aprendizaje pasivo de nombres DNS
	if ev.ResolvedIP != "" && ev.DNSName != "" {
		m.dnsCache[ev.ResolvedIP] = ev.DNSName
		if n, exists := m.nodes[ev.ResolvedIP]; exists && n.hostname == "" {
			n.hostname = ev.DNSName
		}
	}

	srcID := m.mapEndpointID(ev.SrcIP)
	dstID := m.mapEndpointID(ev.DstIP)
	if srcID == dstID {
		return
	}

	proto := ev.Protocol
	if proto == "" {
		proto = "OTHER"
	}
	pktLen := uint64(ev.Length)
	if pktLen == 0 {
		pktLen = 64
	}

	m.packetsSeen++

	srcNode := m.ensureNodeLocked(srcID, ev.SrcIP, ev.SrcMAC, now)
	dstNode := m.ensureNodeLocked(dstID, ev.DstIP, ev.DstMAC, now)

	// Actualizar estadísticas origen (Tx)
	srcNode.winTxBytes += pktLen
	srcNode.winTxPkts++
	srcNode.totalTxBytes += pktLen
	srcNode.totalPackets++
	srcNode.protocols[proto] += pktLen
	srcNode.lastSeen = now
	srcNode.energy = math.Min(1.0, srcNode.energy+0.28)
	if ev.SrcPort > 0 && ev.SrcPort < 49152 {
		srcNode.ports[ev.SrcPort] += pktLen
	}
	srcNode.recordPeer(dstID, pktLen, proto)

	// Actualizar estadísticas destino (Rx)
	dstNode.winRxBytes += pktLen
	dstNode.winRxPkts++
	dstNode.totalRxBytes += pktLen
	dstNode.totalPackets++
	dstNode.protocols[proto] += pktLen
	dstNode.lastSeen = now
	dstNode.energy = math.Min(1.0, dstNode.energy+0.35)
	if ev.DstPort > 0 && ev.DstPort < 49152 {
		dstNode.ports[ev.DstPort] += pktLen
	}
	dstNode.recordPeer(srcID, pktLen, proto)

	// Enlace no dirigido para el grafo (par canónico)
	linkID, a, b := canonicalLink(srcID, dstID)
	link, ok := m.links[linkID]
	if !ok {
		link = &internalLink{
			id:        linkID,
			source:    a,
			target:    b,
			protocols: make(map[string]uint64),
			ports:     make(map[uint16]uint64),
		}
		m.links[linkID] = link
	}
	link.winBytes += pktLen
	link.winPkts++
	link.totalBytes += pktLen
	link.totalPackets++
	link.protocols[proto] += pktLen
	if ev.DstPort > 0 && ev.DstPort < 49152 {
		link.ports[ev.DstPort] += pktLen
	} else if ev.SrcPort > 0 && ev.SrcPort < 49152 {
		link.ports[ev.SrcPort] += pktLen
	}
	link.lastSeen = now

	// Acumular en la ventana de pulsos adaptativos
	fk := flowKey{src: srcID, dst: dstID, protocol: proto}
	acc, ok := m.pulseWindow[fk]
	if !ok {
		acc = &pulseAccum{
			ports: make(map[uint16]struct{}),
			info:  ev.Info,
		}
		m.pulseWindow[fk] = acc
	}
	acc.packets++
	acc.bytes += pktLen
	if ev.DstPort > 0 && ev.DstPort < 49152 {
		acc.ports[ev.DstPort] = struct{}{}
	} else if ev.SrcPort > 0 && ev.SrcPort < 49152 {
		acc.ports[ev.SrcPort] = struct{}{}
	}
	if ev.Info != "" {
		acc.info = ev.Info
	}
}

func (m *Manager) mapEndpointID(ip string) string {
	if m.settings.GroupExternal && !capture.IsPrivateOrLocalIP(ip) {
		return externalGroupID
	}
	return ip
}

func (m *Manager) ensureNodeLocked(id, rawIP, mac string, now time.Time) *internalNode {
	n, ok := m.nodes[id]
	if ok {
		if id != externalGroupID && mac != "" && mac != "00:00:00:00:00:00" && mac != "ff:ff:ff:ff:ff:ff" && n.mac == "" {
			n.mac = mac
			n.vendor = capture.LookupVendor(mac)
		}
		if n.hostname == "" {
			if name, found := m.dnsCache[id]; found {
				n.hostname = name
			}
		}
		return n
	}

	role := "lan"
	displayIP := rawIP
	hostname := m.dnsCache[id]
	vendor := ""

	if id == externalGroupID {
		role = "external"
		displayIP = externalGroupID
		hostname = "Internet (Externo)"
		mac = ""
	} else {
		if m.hostIPs[id] {
			role = "host"
		} else if id == m.gatewayIP && m.gatewayIP != "" {
			role = "gateway"
		} else if !capture.IsPrivateOrLocalIP(id) {
			role = "external"
		}

		if mac == "00:00:00:00:00:00" || mac == "ff:ff:ff:ff:ff:ff" {
			mac = ""
		}
		if mac != "" {
			vendor = capture.LookupVendor(mac)
		}
		if hostname == "" && m.settings.CaptureMode == "live" {
			m.scheduleReverseDNSLocked(id)
		}
	}

	n = &internalNode{
		id:        id,
		ip:        displayIP,
		mac:       mac,
		vendor:    vendor,
		hostname:  hostname,
		role:      role,
		protocols: make(map[string]uint64),
		ports:     make(map[uint16]uint64),
		peers:     make(map[string]*peerAccum),
		lastSeen:  now,
		energy:    0.85,
	}
	m.nodes[id] = n
	return n
}

func (m *Manager) scheduleReverseDNSLocked(ip string) {
	if m.dnsInFlight[ip] {
		return
	}
	if _, exists := m.dnsCache[ip]; exists {
		return
	}
	m.dnsInFlight[ip] = true

	go func(target string) {
		ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
		defer cancel()
		names, err := net.DefaultResolver.LookupAddr(ctx, target)

		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.dnsInFlight, target)
		if err == nil && len(names) > 0 {
			name := strings.TrimSuffix(names[0], ".")
			m.dnsCache[target] = name
			if node, ok := m.nodes[target]; ok && node.hostname == "" {
				node.hostname = name
			}
		} else {
			m.dnsCache[target] = ""
		}
	}(ip)
}

// FlushWindow consolida las métricas de los últimos 100 ms, genera pulsos adaptativos y limpia nodos expirados.
func (m *Manager) FlushWindow(now time.Time) models.TickMessage {
	m.mu.Lock()
	defer m.mu.Unlock()

	dtSec := tickInterval.Seconds()
	const alpha = 0.35

	var totalBps, totalPps float64
	ttl := time.Duration(m.settings.NodeTTLSec) * time.Second

	// 1. Actualizar tasas de nodos y limpiar expirados no fijados
	for id, n := range m.nodes {
		custom, hasCustom := m.customs[id]
		isPinned := hasCustom && custom.Pinned
		if !isPinned && n.role != "host" && now.Sub(n.lastSeen) > ttl {
			delete(m.nodes, id)
			continue
		}

		instRxBps := float64(n.winRxBytes*8) / dtSec
		instTxBps := float64(n.winTxBytes*8) / dtSec
		instRxPps := float64(n.winRxPkts) / dtSec
		instTxPps := float64(n.winTxPkts) / dtSec

		n.rxBps = smoothRate(n.rxBps, instRxBps, alpha)
		n.txBps = smoothRate(n.txBps, instTxBps, alpha)
		n.rxPps = smoothRate(n.rxPps, instRxPps, alpha)
		n.txPps = smoothRate(n.txPps, instTxPps, alpha)

		if n.winRxPkts == 0 && n.winTxPkts == 0 {
			n.energy *= 0.88
			if n.energy < 0.02 {
				n.energy = 0
			}
		}

		n.winRxBytes = 0
		n.winTxBytes = 0
		n.winRxPkts = 0
		n.winTxPkts = 0

		totalBps += n.rxBps
		totalPps += n.rxPps
	}

	// 2. Actualizar tasas de enlaces y remover enlaces huérfanos o expirados
	for id, l := range m.links {
		if _, ok := m.nodes[l.source]; !ok {
			delete(m.links, id)
			continue
		}
		if _, ok := m.nodes[l.target]; !ok {
			delete(m.links, id)
			continue
		}
		if now.Sub(l.lastSeen) > ttl {
			delete(m.links, id)
			continue
		}

		instBps := float64(l.winBytes*8) / dtSec
		instPps := float64(l.winPkts) / dtSec
		l.bps = smoothRate(l.bps, instBps, alpha)
		l.pps = smoothRate(l.pps, instPps, alpha)
		l.winBytes = 0
		l.winPkts = 0
	}

	// 3. Construir pulsos adaptativos para la ventana
	pulses := make([]models.PulseEvent, 0, len(m.pulseWindow))
	for fk, acc := range m.pulseWindow {
		if _, ok := m.nodes[fk.src]; !ok {
			continue
		}
		if _, ok := m.nodes[fk.dst]; !ok {
			continue
		}
		intensity, speed, pCount := computeAdaptivePulse(acc.packets, acc.bytes)
		ports := make([]uint16, 0, len(acc.ports))
		for p := range acc.ports {
			ports = append(ports, p)
		}
		sort.Slice(ports, func(i, j int) bool { return ports[i] < ports[j] })
		if len(ports) > 6 {
			ports = ports[:6]
		}

		seq := m.pulseSeq.Add(1)
		pulses = append(pulses, models.PulseEvent{
			ID:            fmt.Sprintf("p-%d", seq),
			Src:           fk.src,
			Dst:           fk.dst,
			Protocol:      fk.protocol,
			Packets:       acc.packets,
			Bytes:         acc.bytes,
			Intensity:     intensity,
			Speed:         speed,
			ParticleCount: pCount,
			Ports:         ports,
			Info:          acc.info,
		})
	}
	m.pulseWindow = make(map[flowKey]*pulseAccum)

	// Ordenar pulsos por intensidad descendente y limitar por frame para proteger GPU
	sort.Slice(pulses, func(i, j int) bool {
		return pulses[i].Bytes > pulses[j].Bytes
	})
	if len(pulses) > 80 {
		pulses = pulses[:80]
	}

	return m.buildSnapshotLocked("tick", now, totalBps, totalPps, pulses)
}

// GetStateSnapshot devuelve el estado completo actual (usado al conectar WebSocket y en /api/state).
func (m *Manager) GetStateSnapshot() models.StateResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var totalBps, totalPps float64
	for _, n := range m.nodes {
		totalBps += n.rxBps
		totalPps += n.rxPps
	}

	tick := m.buildSnapshotLocked("init", time.Now(), totalBps, totalPps, nil)
	return models.StateResponse{
		Stats:      tick.Stats,
		Nodes:      tick.Nodes,
		Links:      tick.Links,
		Interfaces: append([]models.NetworkInterface(nil), m.interfaces...),
		Settings:   m.settings,
	}
}

// GetInitMessage devuelve el TickMessage de tipo "init" para clientes recién conectados por WS.
func (m *Manager) GetInitMessage() models.TickMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var totalBps, totalPps float64
	for _, n := range m.nodes {
		totalBps += n.rxBps
		totalPps += n.rxPps
	}
	return m.buildSnapshotLocked("init", time.Now(), totalBps, totalPps, []models.PulseEvent{})
}

func (m *Manager) buildSnapshotLocked(msgType string, now time.Time, totalBps, totalPps float64, pulses []models.PulseEvent) models.TickMessage {
	nodesOut := make([]models.NodeInfo, 0, len(m.nodes))
	for _, n := range m.nodes {
		custom := m.customs[n.id]
		alias := custom.Alias
		if alias == "" && n.id == externalGroupID {
			alias = "Internet (Externo)"
		}
		category := custom.Category
		if category == "" {
			category = inferCategory(n.role)
		}

		protoCopy := make(map[string]uint64, len(n.protocols))
		for k, v := range n.protocols {
			protoCopy[k] = v
		}

		nodeInfo := models.NodeInfo{
			ID:           n.id,
			IP:           n.ip,
			MAC:          n.mac,
			Vendor:       n.vendor,
			Hostname:     n.hostname,
			Alias:        alias,
			Category:     category,
			Role:         n.role,
			RxBps:        math.Round(n.rxBps*10) / 10,
			TxBps:        math.Round(n.txBps*10) / 10,
			RxPps:        math.Round(n.rxPps*10) / 10,
			TxPps:        math.Round(n.txPps*10) / 10,
			TotalRxBytes: n.totalRxBytes,
			TotalTxBytes: n.totalTxBytes,
			TotalPackets: n.totalPackets,
			Protocols:    protoCopy,
			ActivePorts:  topPorts(n.ports, 10),
			ActivePeers:  m.topPeersLocked(n.peers, 8),
			Pinned:       custom.Pinned,
			FX:           custom.FX,
			FY:           custom.FY,
			FZ:           custom.FZ,
			LastSeen:     n.lastSeen.UnixMilli(),
			Energy:       math.Round(n.energy*1000) / 1000,
		}
		nodesOut = append(nodesOut, nodeInfo)
	}
	sort.Slice(nodesOut, func(i, j int) bool { return nodesOut[i].ID < nodesOut[j].ID })

	linksOut := make([]models.LinkInfo, 0, len(m.links))
	for _, l := range m.links {
		protoCopy := make(map[string]uint64, len(l.protocols))
		domProto := "OTHER"
		var maxBytes uint64
		for k, v := range l.protocols {
			protoCopy[k] = v
			if v >= maxBytes {
				maxBytes = v
				domProto = k
			}
		}
		linksOut = append(linksOut, models.LinkInfo{
			ID:               l.id,
			Source:           l.source,
			Target:           l.target,
			Bps:              math.Round(l.bps*10) / 10,
			Pps:              math.Round(l.pps*10) / 10,
			TotalBytes:       l.totalBytes,
			TotalPackets:     l.totalPackets,
			DominantProtocol: domProto,
			Protocols:        protoCopy,
			Ports:            topPorts(l.ports, 8),
			LastSeen:         l.lastSeen.UnixMilli(),
		})
	}
	sort.Slice(linksOut, func(i, j int) bool { return linksOut[i].ID < linksOut[j].ID })

	if pulses == nil {
		pulses = []models.PulseEvent{}
	}

	return models.TickMessage{
		Type:      msgType,
		Timestamp: now.UnixMilli(),
		Stats: models.CaptureStats{
			TotalBps:     math.Round(totalBps*10) / 10,
			TotalPps:     math.Round(totalPps*10) / 10,
			ActiveNodes:  len(nodesOut),
			ActiveLinks:  len(linksOut),
			CaptureMode:  m.settings.CaptureMode,
			Interface:    m.settings.Interface,
			CaptureError: m.captureErr,
			PacketsSeen:  m.packetsSeen,
		},
		Nodes:  nodesOut,
		Links:  linksOut,
		Pulses: pulses,
	}
}

// SwitchCaptureMode cambia entre modo "live" (en una interfaz) y "simulation".
func (m *Manager) SwitchCaptureMode(mode, iface string) error {
	if mode != "live" && mode != "simulation" {
		return fmt.Errorf("modo inválido %q: debe ser 'live' o 'simulation'", mode)
	}

	m.mu.Lock()
	modeChanged := m.settings.CaptureMode != mode || (mode == "live" && iface != "" && iface != m.settings.Interface)
	m.settings.CaptureMode = mode
	if mode == "live" && iface != "" {
		m.settings.Interface = iface
	}
	if modeChanged {
		// Reiniciar topología en memoria para no mezclar nodos reales y simulados
		m.nodes = make(map[string]*internalNode)
		m.links = make(map[string]*internalLink)
		m.pulseWindow = make(map[flowKey]*pulseAccum)
		m.packetsSeen = 0
		m.dnsCache = make(map[string]string)
	}
	if m.store != nil {
		_ = m.store.SaveSettings(m.settings)
	}
	m.mu.Unlock()

	if modeChanged {
		m.startSource()
	}
	return nil
}

// ClearTopology reinicia la topología en memoria (mantiene personalizaciones guardadas en SQLite).
func (m *Manager) ClearTopology() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodes = make(map[string]*internalNode)
	m.links = make(map[string]*internalLink)
	m.pulseWindow = make(map[flowKey]*pulseAccum)
	m.packetsSeen = 0
}

// RefreshInterfaces vuelve a listar las interfaces del host.
func (m *Manager) RefreshInterfaces() []models.NetworkInterface {
	ifaces, err := capture.ListInterfaces()
	if err != nil {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return append([]models.NetworkInterface(nil), m.interfaces...)
	}
	m.mu.Lock()
	m.interfaces = ifaces
	m.mu.Unlock()
	return ifaces
}

// SaveNodeCustomization guarda alias, categoría y fijación de posición en SQLite.
func (m *Manager) SaveNodeCustomization(c models.NodeCustomization) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if n, ok := m.nodes[c.IP]; ok && c.MAC == "" {
		c.MAC = n.mac
	}
	if m.store != nil {
		if err := m.store.UpsertNodeCustomization(c); err != nil {
			return err
		}
	}
	m.customs[c.IP] = c
	return nil
}

// DeleteNodeCustomization elimina alias y pin de un nodo.
func (m *Manager) DeleteNodeCustomization(ip string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.store != nil {
		if err := m.store.DeleteNodeCustomization(ip); err != nil {
			return err
		}
	}
	delete(m.customs, ip)
	return nil
}

// UpdateSettings actualiza preferencias globales (agrupación de externos, TTL, bloom).
func (m *Manager) UpdateSettings(groupExternal *bool, nodeTTLSec *int, bloomStrength *float64) (models.AppSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	groupChanged := false
	if groupExternal != nil && *groupExternal != m.settings.GroupExternal {
		m.settings.GroupExternal = *groupExternal
		groupChanged = true
	}
	if nodeTTLSec != nil && *nodeTTLSec >= 10 && *nodeTTLSec <= 3600 {
		m.settings.NodeTTLSec = *nodeTTLSec
	}
	if bloomStrength != nil && *bloomStrength >= 0 && *bloomStrength <= 3.0 {
		m.settings.BloomStrength = *bloomStrength
	}

	if groupChanged {
		// Limpiar enlaces/nodos externos para re-mapear limpiamente la vista agrupada/individual
		m.nodes = make(map[string]*internalNode)
		m.links = make(map[string]*internalLink)
		m.pulseWindow = make(map[flowKey]*pulseAccum)
	}

	if m.store != nil {
		if err := m.store.SaveSettings(m.settings); err != nil {
			return m.settings, err
		}
	}
	return m.settings, nil
}

// computeAdaptivePulse convierte los paquetes y bytes de una ventana en parámetros visuales del pulso.
func computeAdaptivePulse(packets, bytes uint64) (intensity float64, speed float64, particleCount int) {
	if packets <= 1 {
		return 0.28, 1.0, 1
	}
	if packets <= 5 {
		return 0.50, 1.3, 2
	}
	logP := math.Log10(float64(packets))
	logB := math.Log10(math.Max(1, float64(bytes)/1024.0))

	intensity = clamp(0.55+(logP-0.7)*0.22+logB*0.08, 0.55, 1.0)
	speed = clamp(1.35+(logP-0.7)*0.45, 1.35, 2.4)
	particleCount = int(clamp(math.Round(2.0+logP), 3, 5))
	return math.Round(intensity*100)/100, math.Round(speed*100)/100, particleCount
}

func smoothRate(prev, instant, alpha float64) float64 {
	next := prev*(1-alpha) + instant*alpha
	if next < 1.0 && instant == 0 {
		return 0
	}
	return next
}

func clamp(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func canonicalLink(a, b string) (string, string, string) {
	if a <= b {
		return a + "<->" + b, a, b
	}
	return b + "<->" + a, b, a
}

func inferCategory(role string) string {
	switch role {
	case "host":
		return "server"
	case "gateway":
		return "router"
	case "external":
		return "cloud"
	default:
		return "default"
	}
}

func (n *internalNode) recordPeer(peerID string, bytes uint64, proto string) {
	p, ok := n.peers[peerID]
	if !ok {
		p = &peerAccum{ip: peerID}
		n.peers[peerID] = p
	}
	p.bytes += bytes
	p.protocol = proto
}

func (m *Manager) topPeersLocked(peers map[string]*peerAccum, limit int) []models.PeerStat {
	out := make([]models.PeerStat, 0, len(peers))
	for _, p := range peers {
		label := p.ip
		if custom, ok := m.customs[p.ip]; ok && custom.Alias != "" {
			label = custom.Alias
		} else if p.ip == externalGroupID {
			label = "Internet (Externo)"
		} else if node, ok := m.nodes[p.ip]; ok && node.hostname != "" {
			label = node.hostname
		}
		out = append(out, models.PeerStat{
			IP:       p.ip,
			Label:    label,
			Bytes:    p.bytes,
			Protocol: p.protocol,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func topPorts(ports map[uint16]uint64, limit int) []uint16 {
	type kv struct {
		port  uint16
		bytes uint64
	}
	list := make([]kv, 0, len(ports))
	for p, b := range ports {
		list = append(list, kv{port: p, bytes: b})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].bytes > list[j].bytes })
	if len(list) > limit {
		list = list[:limit]
	}
	res := make([]uint16, len(list))
	for i, item := range list {
		res[i] = item.port
	}
	return res
}
