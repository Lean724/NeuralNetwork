import { describe, expect, it } from 'vitest';
import {
  formatBps,
  formatBytes,
  getNodeDisplayName,
  getProtocolColor,
  linkMatchesProtocols,
  matchesNodeSearch
} from './format';
import { LinkInfo, NodeInfo } from '../types/network';

describe('format helpers', () => {
  it('formats bps across scales cleanly', () => {
    expect(formatBps(0)).toBe('0 bps');
    expect(formatBps(640)).toBe('640 bps');
    expect(formatBps(12500)).toBe('12.5 Kbps');
    expect(formatBps(5_400_000)).toBe('5.40 Mbps');
  });

  it('formats byte totals', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(2048)).toBe('2.00 KB');
    expect(formatBytes(5 * 1024 * 1024)).toBe('5.00 MB');
  });

  it('resolves protocol color with fallback', () => {
    expect(getProtocolColor('DNS')).toBe('#fbbf24');
    expect(getProtocolColor('UNKNOWN_PROTO')).toBe('#a78bfa');
  });

  it('prioritizes alias over hostname and IP for display name', () => {
    expect(getNodeDisplayName({ alias: 'NAS Casa', hostname: 'nas.lan', ip: '192.168.1.10' })).toBe('NAS Casa');
    expect(getNodeDisplayName({ alias: '', hostname: 'nas.lan', ip: '192.168.1.10' })).toBe('nas.lan');
    expect(getNodeDisplayName({ alias: '', hostname: '', ip: '192.168.1.10' })).toBe('192.168.1.10');
  });

  it('matches node search across IP, alias, hostname, MAC and vendor', () => {
    const sample: NodeInfo = {
      id: '192.168.1.50',
      ip: '192.168.1.50',
      mac: 'dc:a6:32:4a:11:08',
      vendor: 'Raspberry Pi Trading',
      hostname: 'homelab.local',
      alias: 'Docker Server',
      category: 'server',
      role: 'host',
      rxBps: 0,
      txBps: 0,
      rxPps: 0,
      txPps: 0,
      totalRxBytes: 0,
      totalTxBytes: 0,
      totalPackets: 0,
      protocols: {},
      activePorts: [],
      activePeers: [],
      pinned: false,
      lastSeen: Date.now(),
      energy: 0
    };

    expect(matchesNodeSearch(sample, 'docker')).toBe(true);
    expect(matchesNodeSearch(sample, 'raspberry')).toBe(true);
    expect(matchesNodeSearch(sample, 'dc:a6')).toBe(true);
    expect(matchesNodeSearch(sample, '192.168.1.50')).toBe(true);
    expect(matchesNodeSearch(sample, 'smart-tv')).toBe(false);
  });

  it('filters links according to enabled protocols', () => {
    const link: LinkInfo = {
      id: 'a<->b',
      source: 'a',
      target: 'b',
      bps: 1000,
      pps: 10,
      totalBytes: 500,
      totalPackets: 5,
      dominantProtocol: 'DNS',
      protocols: { DNS: 500 },
      ports: [53],
      lastSeen: Date.now()
    };

    expect(linkMatchesProtocols(link, { DNS: true, TCP: false })).toBe(true);
    expect(linkMatchesProtocols(link, { DNS: false, TCP: true })).toBe(false);
    expect(linkMatchesProtocols(link, { DNS: false, TCP: false })).toBe(false);
  });
});
