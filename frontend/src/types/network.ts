export type ProtocolName =
  | 'TCP'
  | 'HTTPS'
  | 'HTTP'
  | 'UDP'
  | 'DNS'
  | 'ICMP'
  | 'ARP'
  | 'OTHER';

export interface PeerStat {
  ip: string;
  label: string;
  bytes: number;
  protocol: string;
}

export interface NodeInfo {
  id: string;
  ip: string;
  mac: string;
  vendor: string;
  hostname: string;
  alias: string;
  category: string;
  role: 'host' | 'gateway' | 'lan' | 'external' | string;
  rxBps: number;
  txBps: number;
  rxPps: number;
  txPps: number;
  totalRxBytes: number;
  totalTxBytes: number;
  totalPackets: number;
  protocols: Record<string, number>;
  activePorts: number[];
  activePeers: PeerStat[];
  pinned: boolean;
  fx?: number;
  fy?: number;
  fz?: number;
  lastSeen: number;
  energy: number;
  // Coordenadas vivas gestionadas por d3-force-3d
  x?: number;
  y?: number;
  z?: number;
  vx?: number;
  vy?: number;
  vz?: number;
}

export interface LinkInfo {
  id: string;
  source: string | NodeInfo;
  target: string | NodeInfo;
  bps: number;
  pps: number;
  totalBytes: number;
  totalPackets: number;
  dominantProtocol: string;
  protocols: Record<string, number>;
  ports: number[];
  lastSeen: number;
}

export interface PulseEvent {
  id: string;
  src: string;
  dst: string;
  protocol: string;
  packets: number;
  bytes: number;
  intensity: number;
  speed: number;
  particleCount: number;
  ports: number[];
  info?: string;
}

export interface CaptureStats {
  totalBps: number;
  totalPps: number;
  activeNodes: number;
  activeLinks: number;
  captureMode: 'live' | 'simulation';
  interface: string;
  captureError?: string;
  packetsSeen: number;
}

export interface NetworkInterface {
  name: string;
  description: string;
  addresses: string[];
  isLoopback: boolean;
}

export interface AppSettings {
  groupExternal: boolean;
  nodeTtlSec: number;
  bloomStrength: number;
  captureMode: 'live' | 'simulation';
  interface: string;
}

export interface TickMessage {
  type: 'init' | 'tick';
  timestamp: number;
  stats: CaptureStats;
  nodes: NodeInfo[];
  links: LinkInfo[];
  pulses: PulseEvent[];
}

export interface StateResponse {
  stats: CaptureStats;
  nodes: NodeInfo[];
  links: LinkInfo[];
  interfaces: NetworkInterface[];
  settings: AppSettings;
}
