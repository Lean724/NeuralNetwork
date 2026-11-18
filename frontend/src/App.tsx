import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ControlBar } from './components/ControlBar';
import { HudStats } from './components/HudStats';
import { InspectorPanel } from './components/InspectorPanel';
import { NeuralGraph3D } from './components/NeuralGraph3D';
import { useNeuralSocket } from './hooks/useNeuralSocket';
import { NodeInfo, PulseEvent } from './types/network';
import { ALL_PROTOCOLS, matchesNodeSearch } from './utils/format';

// En primera instancia, ningún filtro de protocolo está preseleccionado
const initialProtocols = ALL_PROTOCOLS.reduce<Record<string, boolean>>((acc, p) => {
  acc[p] = false;
  return acc;
}, {});

export const App: React.FC = () => {
  const [enabledProtocols, setEnabledProtocols] = useState<Record<string, boolean>>(initialProtocols);
  const [searchQuery, setSearchQuery] = useState('');
  const [paused, setPaused] = useState(false);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [selectedLinkId, setSelectedLinkId] = useState<string | null>(null);
  const [focusNodeId, setFocusNodeId] = useState<string | null>(null);
  const [focusSeq, setFocusSeq] = useState(0);
  const [resetCameraSeq, setResetCameraSeq] = useState(0);
  const [filterActivationTime, setFilterActivationTime] = useState<number | null>(null);
  const [connectingProgress, setConnectingProgress] = useState<{
    connected: number;
    total: number;
    remainingSec: number;
  } | null>(null);

  const pulseQueueRef = useRef<PulseEvent[]>([]);
  const enabledProtocolsRef = useRef(enabledProtocols);
  enabledProtocolsRef.current = enabledProtocols;

  const handleIncomingPulses = useCallback((incoming: PulseEvent[]) => {
    const activeMap = enabledProtocolsRef.current;
    const filtered = incoming.filter((p) => activeMap[p.protocol] !== false);
    if (filtered.length === 0) return;

    const q = pulseQueueRef.current;
    q.push(...filtered);
    if (q.length > 240) {
      q.splice(0, q.length - 240);
    }
  }, []);

  const {
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
    clearTopology
  } = useNeuralSocket({
    paused,
    onPulses: handleIncomingPulses
  });

  const nodesById = useMemo(() => {
    const map = new Map<string, NodeInfo>();
    for (const n of nodes) {
      map.set(n.id, n);
    }
    return map;
  }, [nodes]);

  const selectedNode = useMemo(
    () => (selectedNodeId ? nodesById.get(selectedNodeId) ?? null : null),
    [selectedNodeId, nodesById]
  );

  const selectedLink = useMemo(
    () => (selectedLinkId ? links.find((l) => l.id === selectedLinkId) ?? null : null),
    [selectedLinkId, links]
  );

  const handleToggleProtocol = (proto: string) => {
    setEnabledProtocols((prev) => {
      const willBeActive = !prev[proto];
      const next = {
        ...prev,
        [proto]: willBeActive
      };
      const anyActive = Object.values(next).some(Boolean);
      if (!anyActive) {
        setFilterActivationTime(null);
        setConnectingProgress(null);
      } else if (willBeActive) {
        // Al seleccionar un chipfilter, inicia la animación de 10s de conexión neuronal
        setFilterActivationTime(performance.now());
      }
      return next;
    });
  };

  // Progreso visual de conexión progresiva (10s / N neuronas)
  useEffect(() => {
    if (filterActivationTime === null) {
      setConnectingProgress(null);
      return;
    }

    const interval = setInterval(() => {
      const elapsed = performance.now() - filterActivationTime;
      const total = nodes.length;
      if (total === 0 || elapsed >= 10000) {
        setConnectingProgress(null);
        return;
      }
      const stepMs = 10000 / total;
      const connected = Math.min(total, Math.floor(elapsed / stepMs) + 1);
      const remainingSec = Math.max(1, Math.ceil((10000 - elapsed) / 1000));
      setConnectingProgress({ connected, total, remainingSec });
    }, 120);

    return () => clearInterval(interval);
  }, [filterActivationTime, nodes.length]);

  const handleFocusHost = useCallback(() => {
    const hostNode =
      nodes.find((n) => n.role === 'host') ||
      nodes.find((n) => n.role === 'gateway') ||
      nodes[0];
    if (hostNode) {
      setSelectedLinkId(null);
      setSelectedNodeId(hostNode.id);
      setFocusNodeId(hostNode.id);
      setFocusSeq((s) => s + 1);
    }
  }, [nodes]);

  const handleSearchSubmit = () => {
    const q = searchQuery.trim();
    if (!q) return;
    const found = nodes.find((n) => matchesNodeSearch(n, q));
    if (found) {
      setSelectedLinkId(null);
      setSelectedNodeId(found.id);
      setFocusNodeId(found.id);
      setFocusSeq((s) => s + 1);
    }
  };

  const handleBloomChange = (val: number) => {
    setSettings((prev) => ({ ...prev, bloomStrength: val }));
    void updateSettings({ bloomStrength: val });
  };

  const handleNodeDragPersist = useCallback(
    (node: NodeInfo, x: number, y: number, z: number) => {
      void saveNodeCustomization({
        ip: node.ip,
        mac: node.mac,
        alias: node.alias,
        category: node.category || 'default',
        pinned: true,
        fx: Math.round(x * 100) / 100,
        fy: Math.round(y * 100) / 100,
        fz: Math.round(z * 100) / 100
      });
    },
    [saveNodeCustomization]
  );

  return (
    <div className="app-shell">
      <NeuralGraph3D
        nodes={nodes}
        links={links}
        enabledProtocols={enabledProtocols}
        searchQuery={searchQuery}
        selectedNodeId={selectedNodeId}
        selectedLinkId={selectedLinkId}
        bloomStrength={settings.bloomStrength}
        paused={paused}
        focusNodeId={focusNodeId}
        focusSeq={focusSeq}
        resetCameraSeq={resetCameraSeq}
        filterActivationTime={filterActivationTime}
        pulseQueueRef={pulseQueueRef}
        onSelectNode={(n) => setSelectedNodeId(n ? n.id : null)}
        onSelectLink={(l) => setSelectedLinkId(l ? l.id : null)}
        onNodeDragPersist={handleNodeDragPersist}
      />

      <ControlBar
        stats={stats}
        interfaces={interfaces}
        settings={settings}
        enabledProtocols={enabledProtocols}
        searchQuery={searchQuery}
        paused={paused}
        onToggleProtocol={handleToggleProtocol}
        onSearchChange={setSearchQuery}
        onSearchSubmit={handleSearchSubmit}
        onSwitchCapture={(mode, iface) => void switchCaptureMode(mode, iface)}
        onToggleGroupExternal={() => void updateSettings({ groupExternal: !settings.groupExternal })}
        onBloomChange={handleBloomChange}
        onTogglePause={() => setPaused((p) => !p)}
        onResetCamera={() => {
          setFocusNodeId(null);
          setResetCameraSeq((s) => s + 1);
        }}
        onFocusHost={handleFocusHost}
        onClearTopology={() => {
          pulseQueueRef.current = [];
          setSelectedNodeId(null);
          setSelectedLinkId(null);
          void clearTopology();
        }}
        connectingProgress={connectingProgress}
      />

      <InspectorPanel
        node={selectedNode}
        link={selectedLink}
        nodesById={nodesById}
        onClose={() => {
          setSelectedNodeId(null);
          setSelectedLinkId(null);
        }}
        onSelectNodeById={(id) => {
          setSelectedLinkId(null);
          setSelectedNodeId(id);
          setFocusNodeId(id);
        }}
        onSaveNode={saveNodeCustomization}
        onResetNodeCustomization={deleteNodeCustomization}
      />

      <HudStats
        status={status}
        stats={stats}
        onActivateSimulation={() => void switchCaptureMode('simulation')}
      />
    </div>
  );
};

export default App;
