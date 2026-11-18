import React from 'react';
import { AlertTriangle, PlayCircle, Wifi } from 'lucide-react';
import { ConnectionStatus } from '../hooks/useNeuralSocket';
import { CaptureStats } from '../types/network';
import { ALL_PROTOCOLS, formatBps, formatPps, getProtocolColor } from '../utils/format';

interface HudStatsProps {
  status: ConnectionStatus;
  stats: CaptureStats;
  onActivateSimulation: () => void;
}

export const HudStats: React.FC<HudStatsProps> = ({ status, stats, onActivateSimulation }) => {
  const showEmptyLiveHint =
    stats.captureMode === 'live' && stats.activeNodes === 0 && !stats.captureError;

  return (
    <div className="hud-bottom">
      {(stats.captureError || showEmptyLiveHint) && (
        <div className="hud-notice">
          <AlertTriangle size={16} />
          <div className="hud-notice-text">
            {stats.captureError ? (
              <span>
                Error capturando en <strong>{stats.interface}</strong>: {stats.captureError}
              </span>
            ) : (
              <span>
                Esperando paquetes en <strong>{stats.interface}</strong> (tráfico en reposo o aislado en VM Docker Desktop).
              </span>
            )}
          </div>
          <button type="button" className="btn-demo" onClick={onActivateSimulation}>
            <PlayCircle size={14} />
            Probar Modo Simulación
          </button>
        </div>
      )}

      <div className="hud-stats-row">
        <div className="hud-pill status-pill">
          <span className={`conn-dot is-${status}`} />
          <Wifi size={14} />
          <span>
            {status === 'connected'
              ? stats.captureMode === 'simulation'
                ? 'Simulación Activa'
                : `Vivo · ${stats.interface}`
              : status === 'connecting'
              ? 'Conectando...'
              : 'Reconectando...'}
          </span>
          <span className="nav-hint" title="WASD o Flechas para moverte, Z/C para Zoom In/Out, Q/E vertical, Shift para acelerar">
            WASD · Z/C Zoom
          </span>
        </div>

        <div className="hud-metrics">
          <div className="hud-metric">
            <span>Neuronas</span>
            <strong>{stats.activeNodes}</strong>
          </div>
          <div className="hud-metric">
            <span>Sinapsis</span>
            <strong>{stats.activeLinks}</strong>
          </div>
          <div className="hud-metric">
            <span>Tráfico</span>
            <strong>{formatBps(stats.totalBps)}</strong>
          </div>
          <div className="hud-metric">
            <span>Impulsos</span>
            <strong>{formatPps(stats.totalPps)}</strong>
          </div>
        </div>

        <div className="hud-legend" aria-label="Leyenda de protocolos">
          {ALL_PROTOCOLS.map((proto) => (
            <span key={proto} className="legend-item">
              <i style={{ backgroundColor: getProtocolColor(proto) }} />
              {proto}
            </span>
          ))}
        </div>
      </div>
    </div>
  );
};
