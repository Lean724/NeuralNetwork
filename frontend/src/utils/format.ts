import { LinkInfo, NodeInfo, ProtocolName } from '../types/network';

export const ALL_PROTOCOLS: ProtocolName[] = [
  'TCP',
  'HTTPS',
  'HTTP',
  'UDP',
  'DNS',
  'ICMP',
  'ARP',
  'OTHER'
];

export const PROTOCOL_COLORS: Record<string, string> = {
  TCP: '#00f0ff',
  HTTPS: '#3b82f6',
  HTTP: '#38bdf8',
  UDP: '#e879f9',
  DNS: '#fbbf24',
  ICMP: '#34d399',
  ARP: '#fb923c',
  OTHER: '#a78bfa'
};

export const ROLE_COLORS: Record<string, string> = {
  host: '#22d3ee',
  gateway: '#f59e0b',
  lan: '#818cf8',
  external: '#38bdf8'
};

export const ROLE_LABELS: Record<string, string> = {
  host: 'Host Local',
  gateway: 'Gateway / Router',
  lan: 'Dispositivo LAN',
  external: 'Externo / Internet'
};

export function getProtocolColor(protocol: string): string {
  return PROTOCOL_COLORS[protocol] ?? PROTOCOL_COLORS.OTHER;
}

export function getRoleColor(role: string): string {
  return ROLE_COLORS[role] ?? ROLE_COLORS.lan;
}

export function getNodeDisplayName(node: Pick<NodeInfo, 'alias' | 'hostname' | 'ip'>): string {
  if (node.alias && node.alias.trim() !== '') {
    return node.alias.trim();
  }
  if (node.hostname && node.hostname.trim() !== '') {
    return node.hostname.trim();
  }
  return node.ip;
}

export function formatBps(bps: number): string {
  if (!Number.isFinite(bps) || bps <= 0) return '0 bps';
  if (bps < 1000) return `${Math.round(bps)} bps`;
  if (bps < 1_000_000) return `${(bps / 1000).toFixed(1)} Kbps`;
  if (bps < 1_000_000_000) return `${(bps / 1_000_000).toFixed(2)} Mbps`;
  return `${(bps / 1_000_000_000).toFixed(2)} Gbps`;
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let idx = 0;
  let val = bytes;
  while (val >= 1024 && idx < units.length - 1) {
    val /= 1024;
    idx++;
  }
  return `${val < 10 && idx > 0 ? val.toFixed(2) : val.toFixed(1)} ${units[idx]}`;
}

export function formatPps(pps: number): string {
  if (!Number.isFinite(pps) || pps <= 0) return '0 pps';
  if (pps < 10) return `${pps.toFixed(1)} pps`;
  return `${Math.round(pps)} pps`;
}

export function matchesNodeSearch(node: NodeInfo, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  return (
    node.ip.toLowerCase().includes(q) ||
    node.alias.toLowerCase().includes(q) ||
    node.hostname.toLowerCase().includes(q) ||
    node.mac.toLowerCase().includes(q) ||
    node.vendor.toLowerCase().includes(q)
  );
}

export function linkMatchesProtocols(
  link: LinkInfo,
  enabledProtocols: Record<string, boolean>
): boolean {
  const anyActive = Object.values(enabledProtocols).some(Boolean);
  if (!anyActive) {
    return false;
  }
  const protos = Object.keys(link.protocols || {});
  if (protos.length === 0) {
    return Boolean(enabledProtocols[link.dominantProtocol]);
  }
  return protos.some((p) => Boolean(enabledProtocols[p]));
}

export function endpointId(endpoint: string | NodeInfo): string {
  return typeof endpoint === 'string' ? endpoint : endpoint.id;
}
