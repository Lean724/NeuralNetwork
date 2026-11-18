import React, { useEffect, useState } from 'react';
import {
  ArrowDownLeft,
  ArrowUpRight,
  Cpu,
  Pin,
  PinOff,
  RotateCcw,
  Save,
  Share2,
  X
} from 'lucide-react';
import { LinkInfo, NodeInfo } from '../types/network';
import {
  endpointId,
  formatBps,
  formatBytes,
  formatPps,
  getNodeDisplayName,
  getProtocolColor,
  getRoleColor,
  ROLE_LABELS
} from '../utils/format';

interface InspectorPanelProps {
  node: NodeInfo | null;
  link: LinkInfo | null;
  nodesById: Map<string, NodeInfo>;
  onClose: () => void;
  onSelectNodeById: (id: string) => void;
  onSaveNode: (payload: {
    ip: string;
    mac?: string;
    alias: string;
    category: string;
    pinned: boolean;
    fx?: number;
    fy?: number;
    fz?: number;
  }) => Promise<boolean>;
  onResetNodeCustomization: (ip: string) => Promise<boolean>;
}

const CATEGORIES = [
  { value: 'default', label: 'Estándar' },
  { value: 'server', label: 'Servidor' },
  { value: 'router', label: 'Router / Gateway' },
  { value: 'nas', label: 'NAS / Almacenamiento' },
  { value: 'pc', label: 'Estación de trabajo' },
  { value: 'mobile', label: 'Móvil / Laptop' },
  { value: 'iot', label: 'IoT / Sensor' },
  { value: 'cloud', label: 'Servicio Externo' }
];

export const InspectorPanel: React.FC<InspectorPanelProps> = ({
  node,
  link,
  nodesById,
  onClose,
  onSelectNodeById,
  onSaveNode,
  onResetNodeCustomization
}) => {
  const [alias, setAlias] = useState('');
  const [category, setCategory] = useState('default');
  const [pinned, setPinned] = useState(false);
  const [saving, setSaving] = useState(false);
  const [feedback, setFeedback] = useState<string | null>(null);

  useEffect(() => {
    if (!node) return;
    setAlias(node.alias || '');
    setCategory(node.category || 'default');
    setPinned(node.pinned);
    setFeedback(null);
  }, [node?.id, node?.alias, node?.category, node?.pinned]);

  if (!node && !link) return null;

  if (link) {
    const srcId = endpointId(link.source);
    const dstId = endpointId(link.target);
    const srcNode = nodesById.get(srcId);
    const dstNode = nodesById.get(dstId);
    const protoEntries = Object.entries(link.protocols || {}).sort((a, b) => b[1] - a[1]);

    return (
      <aside className="inspector-panel">
        <div className="inspector-header">
          <div className="inspector-title-wrap">
            <Share2 size={18} style={{ color: getProtocolColor(link.dominantProtocol) }} />
            <div>
              <h2>Sinapsis Activa</h2>
              <span className="inspector-sub">Flujo agregado entre neuronas</span>
            </div>
          </div>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="Cerrar panel">
            <X size={16} />
          </button>
        </div>

        <div className="synapse-endpoints">
          <button type="button" className="endpoint-chip" onClick={() => onSelectNodeById(srcId)}>
            <strong>{srcNode ? getNodeDisplayName(srcNode) : srcId}</strong>
            <span>{srcId}</span>
          </button>
          <span className="endpoint-arrow">↔</span>
          <button type="button" className="endpoint-chip" onClick={() => onSelectNodeById(dstId)}>
            <strong>{dstNode ? getNodeDisplayName(dstNode) : dstId}</strong>
            <span>{dstId}</span>
          </button>
        </div>

        <div className="metric-grid">
          <div className="metric-card">
            <span className="metric-label">Ancho de banda vivo</span>
            <strong className="metric-value">{formatBps(link.bps)}</strong>
            <span className="metric-extra">{formatPps(link.pps)}</span>
          </div>
          <div className="metric-card">
            <span className="metric-label">Acumulado sesión</span>
            <strong className="metric-value">{formatBytes(link.totalBytes)}</strong>
            <span className="metric-extra">{link.totalPackets.toLocaleString()} paq.</span>
          </div>
        </div>

        <div className="inspector-block">
          <h3>Protocolos observados</h3>
          <div className="proto-bars">
            {protoEntries.map(([proto, bytes]) => {
              const pct = link.totalBytes > 0 ? (bytes / link.totalBytes) * 100 : 0;
              return (
                <div key={proto} className="proto-bar-row">
                  <div className="proto-bar-meta">
                    <span style={{ color: getProtocolColor(proto) }}>{proto}</span>
                    <span>
                      {formatBytes(bytes)} ({pct.toFixed(0)}%)
                    </span>
                  </div>
                  <div className="proto-bar-track">
                    <div
                      className="proto-bar-fill"
                      style={{ width: `${Math.max(4, pct)}%`, background: getProtocolColor(proto) }}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        {link.ports && link.ports.length > 0 && (
          <div className="inspector-block">
            <h3>Puertos detectados</h3>
            <div className="tag-list">
              {link.ports.map((p) => (
                <span key={p} className="port-tag">
                  :{p}
                </span>
              ))}
            </div>
          </div>
        )}
      </aside>
    );
  }

  if (!node) return null;

  const totalBytes = node.totalRxBytes + node.totalTxBytes;
  const protoEntries = Object.entries(node.protocols || {}).sort((a, b) => b[1] - a[1]);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setFeedback(null);
    const ok = await onSaveNode({
      ip: node.ip,
      mac: node.mac,
      alias: alias.trim(),
      category,
      pinned,
      fx: pinned ? node.fx ?? node.x ?? 0 : undefined,
      fy: pinned ? node.fy ?? node.y ?? 0 : undefined,
      fz: pinned ? node.fz ?? node.z ?? 0 : undefined
    });
    setSaving(false);
    setFeedback(ok ? 'Guardado en SQLite' : 'Error al guardar');
  };

  const handleTogglePinNow = async () => {
    const nextPinned = !pinned;
    setPinned(nextPinned);
    setSaving(true);
    const ok = await onSaveNode({
      ip: node.ip,
      mac: node.mac,
      alias: alias.trim(),
      category,
      pinned: nextPinned,
      fx: nextPinned ? node.x ?? 0 : undefined,
      fy: nextPinned ? node.y ?? 0 : undefined,
      fz: nextPinned ? node.z ?? 0 : undefined
    });
    setSaving(false);
    setFeedback(ok ? (nextPinned ? 'Posición 3D fijada' : 'Posición liberada') : 'Error al actualizar');
  };

  return (
    <aside className="inspector-panel">
      <div className="inspector-header">
        <div className="inspector-title-wrap">
          <Cpu size={18} style={{ color: getRoleColor(node.role) }} />
          <div>
            <h2>{getNodeDisplayName(node)}</h2>
            <span className="role-badge" style={{ borderColor: getRoleColor(node.role), color: getRoleColor(node.role) }}>
              {ROLE_LABELS[node.role] ?? node.role}
            </span>
          </div>
        </div>
        <button type="button" className="icon-btn" onClick={onClose} aria-label="Cerrar panel">
          <X size={16} />
        </button>
      </div>

      <dl className="info-kv">
        <div>
          <dt>Dirección IP</dt>
          <dd className="mono">{node.ip}</dd>
        </div>
        {node.hostname && (
          <div>
            <dt>Hostname</dt>
            <dd className="mono">{node.hostname}</dd>
          </div>
        )}
        {node.mac && (
          <div>
            <dt>Dirección MAC</dt>
            <dd className="mono">{node.mac}</dd>
          </div>
        )}
        {node.vendor && (
          <div>
            <dt>Fabricante</dt>
            <dd>{node.vendor}</dd>
          </div>
        )}
      </dl>

      <div className="metric-grid">
        <div className="metric-card">
          <span className="metric-label">
            <ArrowDownLeft size={13} /> Entrante (Rx)
          </span>
          <strong className="metric-value">{formatBps(node.rxBps)}</strong>
          <span className="metric-extra">
            {formatPps(node.rxPps)} · {formatBytes(node.totalRxBytes)}
          </span>
        </div>
        <div className="metric-card">
          <span className="metric-label">
            <ArrowUpRight size={13} /> Saliente (Tx)
          </span>
          <strong className="metric-value">{formatBps(node.txBps)}</strong>
          <span className="metric-extra">
            {formatPps(node.txPps)} · {formatBytes(node.totalTxBytes)}
          </span>
        </div>
      </div>

      {protoEntries.length > 0 && (
        <div className="inspector-block">
          <h3>Distribución de protocolos</h3>
          <div className="proto-bars">
            {protoEntries.slice(0, 5).map(([proto, bytes]) => {
              const pct = totalBytes > 0 ? (bytes / totalBytes) * 100 : 0;
              return (
                <div key={proto} className="proto-bar-row">
                  <div className="proto-bar-meta">
                    <span style={{ color: getProtocolColor(proto) }}>{proto}</span>
                    <span>{pct.toFixed(0)}%</span>
                  </div>
                  <div className="proto-bar-track">
                    <div
                      className="proto-bar-fill"
                      style={{ width: `${Math.max(4, pct)}%`, background: getProtocolColor(proto) }}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {node.activePorts && node.activePorts.length > 0 && (
        <div className="inspector-block">
          <h3>Puertos activos</h3>
          <div className="tag-list">
            {node.activePorts.map((port) => (
              <span key={port} className="port-tag">
                :{port}
              </span>
            ))}
          </div>
        </div>
      )}

      {node.activePeers && node.activePeers.length > 0 && (
        <div className="inspector-block">
          <h3>Sinapsis con pares ({node.activePeers.length})</h3>
          <div className="peer-list">
            {node.activePeers.map((peer) => (
              <button
                key={peer.ip}
                type="button"
                className="peer-row"
                onClick={() => onSelectNodeById(peer.ip)}
              >
                <span className="peer-name">{peer.label || peer.ip}</span>
                <span className="peer-meta">
                  <span style={{ color: getProtocolColor(peer.protocol) }}>{peer.protocol}</span>
                  <span>{formatBytes(peer.bytes)}</span>
                </span>
              </button>
            ))}
          </div>
        </div>
      )}

      <form className="inspector-block node-custom-form" onSubmit={handleSave}>
        <h3>Personalización persistente</h3>
        <label>
          <span>Alias del equipo</span>
          <input
            type="text"
            maxLength={80}
            value={alias}
            onChange={(e) => setAlias(e.target.value)}
            placeholder="Ej: NAS Principal, Router..."
          />
        </label>
        <label>
          <span>Categoría</span>
          <select value={category} onChange={(e) => setCategory(e.target.value)}>
            {CATEGORIES.map((c) => (
              <option key={c.value} value={c.value}>
                {c.label}
              </option>
            ))}
          </select>
        </label>

        <div className="form-actions">
          <button
            type="button"
            className={`btn-secondary ${pinned ? 'is-active' : ''}`}
            onClick={handleTogglePinNow}
            disabled={saving}
          >
            {pinned ? <Pin size={14} /> : <PinOff size={14} />}
            {pinned ? 'Fijado en 3D' : 'Fijar posición'}
          </button>

          <button type="submit" className="btn-primary" disabled={saving}>
            <Save size={14} />
            Guardar
          </button>

          {(node.alias || node.pinned) && (
            <button
              type="button"
              className="icon-btn"
              title="Restablecer personalización"
              onClick={async () => {
                setSaving(true);
                await onResetNodeCustomization(node.ip);
                setAlias('');
                setPinned(false);
                setSaving(false);
                setFeedback('Personalización eliminada');
              }}
            >
              <RotateCcw size={14} />
            </button>
          )}
        </div>
        {feedback && <p className="form-feedback">{feedback}</p>}
      </form>
    </aside>
  );
};
