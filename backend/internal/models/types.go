package models

import "time"

// PacketEvent representa un paquete de red capturado o simulado con metadatos de cabeceras.
type PacketEvent struct {
	Timestamp time.Time
	SrcIP     string
	DstIP     string
	SrcMAC    string
	DstMAC    string
	Protocol  string
	SrcPort   uint16
	DstPort   uint16
	Length    int
	DNSName   string
	ResolvedIP string
	Info      string
}

// PeerStat resume el tráfico intercambiado con un nodo par en la sesión actual.
type PeerStat struct {
	IP       string `json:"ip"`
	Label    string `json:"label"`
	Bytes    uint64 `json:"bytes"`
	Protocol string `json:"protocol"`
}

// NodeInfo describe una neurona (dispositivo o nodo externo) en el grafo 3D.
type NodeInfo struct {
	ID           string            `json:"id"`
	IP           string            `json:"ip"`
	MAC          string            `json:"mac"`
	Vendor       string            `json:"vendor"`
	Hostname     string            `json:"hostname"`
	Alias        string            `json:"alias"`
	Category     string            `json:"category"`
	Role         string            `json:"role"` // host | gateway | lan | external
	RxBps        float64           `json:"rxBps"`
	TxBps        float64           `json:"txBps"`
	RxPps        float64           `json:"rxPps"`
	TxPps        float64           `json:"txPps"`
	TotalRxBytes uint64            `json:"totalRxBytes"`
	TotalTxBytes uint64            `json:"totalTxBytes"`
	TotalPackets uint64            `json:"totalPackets"`
	Protocols    map[string]uint64 `json:"protocols"`
	ActivePorts  []uint16          `json:"activePorts"`
	ActivePeers  []PeerStat        `json:"activePeers"`
	Pinned       bool              `json:"pinned"`
	FX           *float64          `json:"fx,omitempty"`
	FY           *float64          `json:"fy,omitempty"`
	FZ           *float64          `json:"fz,omitempty"`
	LastSeen     int64             `json:"lastSeen"`
	Energy       float64           `json:"energy"`
}

// LinkInfo describe una sinapsis (conexión agregada entre dos neuronas).
type LinkInfo struct {
	ID               string            `json:"id"`
	Source           string            `json:"source"`
	Target           string            `json:"target"`
	Bps              float64           `json:"bps"`
	Pps              float64           `json:"pps"`
	TotalBytes       uint64            `json:"totalBytes"`
	TotalPackets     uint64            `json:"totalPackets"`
	DominantProtocol string            `json:"dominantProtocol"`
	Protocols        map[string]uint64 `json:"protocols"`
	Ports            []uint16          `json:"ports"`
	LastSeen         int64             `json:"lastSeen"`
}

// PulseEvent representa un pulso o macro-pulso adaptativo que viaja por una sinapsis.
type PulseEvent struct {
	ID            string   `json:"id"`
	Src           string   `json:"src"`
	Dst           string   `json:"dst"`
	Protocol      string   `json:"protocol"`
	Packets       uint64   `json:"packets"`
	Bytes         uint64   `json:"bytes"`
	Intensity     float64  `json:"intensity"`
	Speed         float64  `json:"speed"`
	ParticleCount int      `json:"particleCount"`
	Ports         []uint16 `json:"ports"`
	Info          string   `json:"info,omitempty"`
}

// CaptureStats contiene métricas globales del monitor en tiempo real.
type CaptureStats struct {
	TotalBps     float64 `json:"totalBps"`
	TotalPps     float64 `json:"totalPps"`
	ActiveNodes  int     `json:"activeNodes"`
	ActiveLinks  int     `json:"activeLinks"`
	CaptureMode  string  `json:"captureMode"` // live | simulation
	Interface    string  `json:"interface"`
	CaptureError string  `json:"captureError,omitempty"`
	PacketsSeen  uint64  `json:"packetsSeen"`
}

// TickMessage es el mensaje enviado por WebSocket al cliente en cada ventana de agregación.
type TickMessage struct {
	Type      string       `json:"type"` // init | tick
	Timestamp int64        `json:"timestamp"`
	Stats     CaptureStats `json:"stats"`
	Nodes     []NodeInfo   `json:"nodes"`
	Links     []LinkInfo   `json:"links"`
	Pulses    []PulseEvent `json:"pulses"`
}

// NodeCustomization guarda las preferencias persistentes de un nodo conocido en SQLite.
type NodeCustomization struct {
	IP       string   `json:"ip"`
	MAC      string   `json:"mac"`
	Alias    string   `json:"alias"`
	Category string   `json:"category"`
	Pinned   bool     `json:"pinned"`
	FX       *float64 `json:"fx"`
	FY       *float64 `json:"fy"`
	FZ       *float64 `json:"fz"`
}

// AppSettings contiene las preferencias globales persistentes de la aplicación.
type AppSettings struct {
	GroupExternal bool    `json:"groupExternal"`
	NodeTTLSec    int     `json:"nodeTtlSec"`
	BloomStrength float64 `json:"bloomStrength"`
	CaptureMode   string  `json:"captureMode"`
	Interface     string  `json:"interface"`
}

// NetworkInterface representa una interfaz de red detectada por libpcap en el host.
type NetworkInterface struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Addresses   []string `json:"addresses"`
	IsLoopback  bool     `json:"isLoopback"`
}

// StateResponse es la respuesta consolidada del endpoint REST /api/state.
type StateResponse struct {
	Stats      CaptureStats       `json:"stats"`
	Nodes      []NodeInfo         `json:"nodes"`
	Links      []LinkInfo         `json:"links"`
	Interfaces []NetworkInterface `json:"interfaces"`
	Settings   AppSettings        `json:"settings"`
}
