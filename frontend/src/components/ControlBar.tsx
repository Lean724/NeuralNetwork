import React from 'react';
import {
  Activity,
  Crosshair,
  Globe,
  Pause,
  Play,
  Radio,
  Search,
  Sparkles,
  Trash2
} from 'lucide-react';
import { AppSettings, CaptureStats, NetworkInterface } from '../types/network';
import { ALL_PROTOCOLS, getProtocolColor } from '../utils/format';

interface ControlBarProps {
  stats: CaptureStats;
  interfaces: NetworkInterface[];
  settings: AppSettings;
  enabledProtocols: Record<string, boolean>;
  searchQuery: string;
  paused: boolean;
  onToggleProtocol: (proto: string) => void;
  onSearchChange: (q: string) => void;
  onSearchSubmit: () => void;
  onSwitchCapture: (mode: 'live' | 'simulation', iface?: string) => void;
  onToggleGroupExternal: () => void;
  onBloomChange: (val: number) => void;
  onTogglePause: () => void;
  onResetCamera: () => void;
  onClearTopology: () => void;
}

export const ControlBar: React.FC<ControlBarProps> = ({
  stats,
  interfaces,
  settings,
  enabledProtocols,
  searchQuery,
  paused,
  onToggleProtocol,
  onSearchChange,
  onSearchSubmit,
  onSwitchCapture,
  onToggleGroupExternal,
  onBloomChange,
  onTogglePause,
  onResetCamera,
  onClearTopology
}) => {
  const selectValue = stats.captureMode === 'simulation' ? '__simulation__' : stats.interface;

  const handleSourceChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const val = e.target.value;
    if (val === '__simulation__') {
      onSwitchCapture('simulation');
    } else {
      onSwitchCapture('live', val);
    }
  };

  return (
    <header className="control-bar">
      <div className="brand-group">
        <div className="brand-mark">
          <Activity size={18} />
        </div>
        <div>
          <h1 className="brand-title">NeuralNetwork</h1>
          <span className="brand-subtitle">Sinapsis de Red 3D</span>
        </div>
      </div>

      <div className="control-section">
        <label className="source-select-wrap" title="Fuente de captura de tráfico">
          <Radio size={14} className={stats.captureMode === 'simulation' ? 'icon-sim' : 'icon-live'} />
          <select value={selectValue} onChange={handleSourceChange} aria-label="Fuente de captura">
            <option value="__simulation__">Modo Simulación (Demo 3D)</option>
            {interfaces.map((ifc) => (
              <option key={ifc.name} value={ifc.name}>
                Live: {ifc.name} {ifc.addresses?.[0] ? `(${ifc.addresses[0]})` : ''}
              </option>
            ))}
          </select>
        </label>

        <form
          className="search-wrap"
          onSubmit={(e) => {
            e.preventDefault();
            onSearchSubmit();
          }}
        >
          <Search size={14} />
          <input
            type="text"
            placeholder="Buscar IP, alias o MAC..."
            value={searchQuery}
            onChange={(e) => onSearchChange(e.target.value)}
            aria-label="Buscar nodo"
          />
        </form>
      </div>

      <div className="protocol-pills" role="group" aria-label="Filtros por protocolo">
        {ALL_PROTOCOLS.map((proto) => {
          const active = enabledProtocols[proto] !== false;
          const color = getProtocolColor(proto);
          return (
            <button
              key={proto}
              type="button"
              className={`proto-pill ${active ? 'is-active' : ''}`}
              style={{ '--proto-color': color } as React.CSSProperties}
              onClick={() => onToggleProtocol(proto)}
              title={`Filtrar tráfico ${proto}`}
            >
              <span className="proto-dot" />
              {proto}
            </button>
          );
        })}
      </div>

      <div className="control-actions">
        <button
          type="button"
          className={`action-btn ${settings.groupExternal ? 'is-active' : ''}`}
          onClick={onToggleGroupExternal}
          title="Agrupar IPs públicas de Internet en un solo nodo concentrador"
        >
          <Globe size={15} />
          <span>Agrupar Internet</span>
        </button>

        <label className="bloom-control" title="Intensidad de brillo neuronal (Bloom)">
          <Sparkles size={14} />
          <input
            type="range"
            min="0"
            max="2.5"
            step="0.05"
            value={settings.bloomStrength}
            onChange={(e) => onBloomChange(parseFloat(e.target.value))}
            aria-label="Intensidad Bloom"
          />
        </label>

        <button
          type="button"
          className={`icon-btn ${paused ? 'is-warning' : ''}`}
          onClick={onTogglePause}
          title={paused ? 'Reanudar pulsos en escena' : 'Pausar escena'}
          aria-label={paused ? 'Reanudar' : 'Pausar'}
        >
          {paused ? <Play size={15} /> : <Pause size={15} />}
        </button>

        <button
          type="button"
          className="icon-btn"
          onClick={onResetCamera}
          title="Centrar cámara 3D"
          aria-label="Centrar cámara"
        >
          <Crosshair size={15} />
        </button>

        <button
          type="button"
          className="icon-btn"
          onClick={onClearTopology}
          title="Limpiar nodos efímeros y reiniciar contadores de sesión"
          aria-label="Limpiar escena"
        >
          <Trash2 size={15} />
        </button>
      </div>
    </header>
  );
};
