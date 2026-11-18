import { useCallback, useEffect, useRef, useState } from 'react';
import {
  AppSettings,
  CaptureStats,
  LinkInfo,
  NetworkInterface,
  NodeInfo,
  PulseEvent,
  StateResponse,
  TickMessage
} from '../types/network';

export type ConnectionStatus = 'connecting' | 'connected' | 'reconnecting';

const DEFAULT_STATS: CaptureStats = {
  totalBps: 0,
  totalPps: 0,
  activeNodes: 0,
  activeLinks: 0,
  captureMode: 'live',
  interface: 'auto',
  packetsSeen: 0
};

const DEFAULT_SETTINGS: AppSettings = {
  groupExternal: false,
  nodeTtlSec: 120,
  bloomStrength: 1.15,
  captureMode: 'live',
  interface: 'auto'
};

export interface UseNeuralSocketOptions {
  paused: boolean;
  onPulses: (pulses: PulseEvent[]) => void;
}

export function useNeuralSocket({ paused, onPulses }: UseNeuralSocketOptions) {
  const [status, setStatus] = useState<ConnectionStatus>('connecting');
  const [stats, setStats] = useState<CaptureStats>(DEFAULT_STATS);
  const [nodes, setNodes] = useState<NodeInfo[]>([]);
  const [links, setLinks] = useState<LinkInfo[]>([]);
  const [interfaces, setInterfaces] = useState<NetworkInterface[]>([]);
  const [settings, setSettings] = useState<AppSettings>(DEFAULT_SETTINGS);

  const pausedRef = useRef(paused);
  pausedRef.current = paused;

  const onPulsesRef = useRef(onPulses);
  onPulsesRef.current = onPulses;

  const fetchFullState = useCallback(async () => {
    try {
      const res = await fetch('/api/state');
      if (!res.ok) return;
      const data: StateResponse = await res.json();
      setStats(data.stats);
      setNodes(data.nodes || []);
      setLinks(data.links || []);
      setInterfaces(data.interfaces || []);
      setSettings(data.settings);
    } catch {
      // Silenciado; el bucle WS reintentará la conexión
    }
  }, []);

  useEffect(() => {
    fetchFullState();
  }, [fetchFullState]);

  useEffect(() => {
    let ws: WebSocket | null = null;
    let reconnectTimer: number | null = null;
    let disposed = false;

    const connect = () => {
      if (disposed) return;
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${protocol}//${window.location.host}/ws`;
      setStatus((prev) => (prev === 'connected' ? 'connected' : 'connecting'));

      ws = new WebSocket(wsUrl);
      ws.onopen = () => {
        if (!disposed) {
          setStatus('connected');
        }
      };

      ws.onmessage = (event) => {
        if (disposed || pausedRef.current) return;
        try {
          const msg: TickMessage = JSON.parse(event.data);
          setStats(msg.stats);
          setNodes(msg.nodes || []);
          setLinks(msg.links || []);
          if (msg.pulses && msg.pulses.length > 0) {
            onPulsesRef.current(msg.pulses);
          }
        } catch {
          // Ignorar mensaje malformado
        }
      };

      ws.onclose = () => {
        if (disposed) return;
        setStatus('reconnecting');
        reconnectTimer = window.setTimeout(connect, 1500);
      };

      ws.onerror = () => {
        ws?.close();
      };
    };

    connect();

    return () => {
      disposed = true;
      if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer);
      }
      ws?.close();
    };
  }, []);

  const switchCaptureMode = useCallback(async (mode: 'live' | 'simulation', iface?: string) => {
    const res = await fetch('/api/capture/mode', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode, interface: iface ?? '' })
    });
    if (res.ok) {
      const data: StateResponse = await res.json();
      setStats(data.stats);
      setNodes(data.nodes || []);
      setLinks(data.links || []);
      setInterfaces(data.interfaces || []);
      setSettings(data.settings);
    }
  }, []);

  const updateSettings = useCallback(async (partial: Partial<AppSettings>) => {
    const res = await fetch('/api/settings', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(partial)
    });
    if (res.ok) {
      const updated: AppSettings = await res.json();
      setSettings(updated);
      await fetchFullState();
    }
  }, [fetchFullState]);

  const saveNodeCustomization = useCallback(
    async (payload: {
      ip: string;
      mac?: string;
      alias: string;
      category: string;
      pinned: boolean;
      fx?: number;
      fy?: number;
      fz?: number;
    }) => {
      const encoded = encodeURIComponent(payload.ip);
      const res = await fetch(`/api/nodes/${encoded}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      if (res.ok) {
        await fetchFullState();
      }
      return res.ok;
    },
    [fetchFullState]
  );

  const deleteNodeCustomization = useCallback(
    async (ip: string) => {
      const encoded = encodeURIComponent(ip);
      const res = await fetch(`/api/nodes/${encoded}`, {
        method: 'DELETE'
      });
      if (res.ok) {
        await fetchFullState();
      }
      return res.ok;
    },
    [fetchFullState]
  );

  const clearTopology = useCallback(async () => {
    const res = await fetch('/api/topology/clear', { method: 'POST' });
    if (res.ok) {
      const data: StateResponse = await res.json();
      setStats(data.stats);
      setNodes(data.nodes || []);
      setLinks(data.links || []);
    }
  }, []);

  return {
    status,
    stats,
    nodes,
    links,
    interfaces,
    settings,
    setSettings,
    switchCaptureMode,
    updateSettings,
    saveNodeCustomization,
    deleteNodeCustomization,
    clearTopology,
    refreshState: fetchFullState
  };
}
