import React, { useCallback, useMemo, useRef, useState } from 'react';
import { ControlBar } from './components/ControlBar';
import { HudStats } from './components/HudStats';
import { InspectorPanel } from './components/InspectorPanel';
import { NeuralGraph3D } from './components/NeuralGraph3D';
import { useNeuralSocket } from './hooks/useNeuralSocket';
import { NodeInfo, PulseEvent } from './types/network';
import { ALL_PROTOCOLS, matchesNodeSearch } from './utils/format';

const initialProtocols = ALL_PROTOCOLS.reduce<Record<string, boolean>>((acc, p) => {
  acc[p] = true;
  return acc;
}, {});

export const App: React.FC = () => {
  const [enabledProtocols, setEnabledProtocols] = useState<Record<string, boolean>>(initialProtocols);
  const [searchQuery, setSearchQuery] = useState('');
  const [paused, setPaused] = useState(false);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [selectedLinkId, setSelectedLinkId] = useState<string | null>(null);
  const [focusNodeId, setFocusNodeId] = useState<string | null>(null);
  const [resetCameraSeq, setResetCameraSeq] = useState(0);

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
    setEnabledProtocols((prev) => ({
      ...prev,
      [proto]: prev[proto] === false
    }));
  };

  const handleSearchSubmit = () => {
    const q = searchQuery.trim();
    if (!q) return;
    const found = nodes.find((n) => matchesNodeSearch(n, q));
    if (found) {
      setSelectedLinkId(null);
      setSelectedNodeId(found.id);
      setFocusNodeId(found.id);
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
        resetCameraSeq={resetCameraSeq}
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
        onClearTopology={() => {
          pulseQueueRef.current = [];
          setSelectedNodeId(null);
          setSelectedLinkId(null);
          void clearTopology();
        }}
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
