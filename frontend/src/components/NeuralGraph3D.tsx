import React, { useEffect, useRef, useState } from 'react';
import ForceGraph3D, { ForceGraphMethods } from 'react-force-graph-3d';
import * as THREE from 'three';
import SpriteText from 'three-spritetext';
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js';

import { LinkInfo, NodeInfo, PulseEvent } from '../types/network';
import {
  endpointId,
  formatBps,
  getNodeDisplayName,
  getProtocolColor,
  getRoleColor,
  linkMatchesProtocols,
  matchesNodeSearch
} from '../utils/format';

interface NeuralGraph3DProps {
  nodes: NodeInfo[];
  links: LinkInfo[];
  enabledProtocols: Record<string, boolean>;
  searchQuery: string;
  selectedNodeId: string | null;
  selectedLinkId: string | null;
  bloomStrength: number;
  paused: boolean;
  focusNodeId: string | null;
  resetCameraSeq: number;
  pulseQueueRef: React.MutableRefObject<PulseEvent[]>;
  onSelectNode: (node: NodeInfo | null) => void;
  onSelectLink: (link: LinkInfo | null) => void;
  onNodeDragPersist: (node: NodeInfo, x: number, y: number, z: number) => void;
}

interface NeuronVisual {
  group: THREE.Group;
  core: THREE.Mesh;
  coreMat: THREE.MeshStandardMaterial;
  halo: THREE.Mesh;
  haloMat: THREE.MeshBasicMaterial;
  ring: THREE.Mesh;
  ringMat: THREE.MeshBasicMaterial;
  label: SpriteText;
  lastLabelText: string;
  lastColor: string;
}

interface ActivePulse {
  mesh: THREE.Mesh;
  srcId: string;
  dstId: string;
  linkId: string;
  progress: number;
  delay: number;
  speed: number;
  radius: number;
}

interface LinkWithCurve extends LinkInfo {
  __curve?: { getPoint: (t: number) => THREE.Vector3 };
}

const MAX_ACTIVE_PULSES = 450;

// Geometrías compartidas para mantener rendimiento WebGL óptimo (zero allocations por tick)
const SHARED_CORE_GEO = new THREE.SphereGeometry(4.2, 20, 20);
const SHARED_HALO_GEO = new THREE.SphereGeometry(7.2, 16, 16);
const SHARED_RING_GEO = new THREE.TorusGeometry(8.8, 0.48, 8, 28);
const SHARED_PULSE_GEO = new THREE.SphereGeometry(1, 10, 10);

const PULSE_MATERIALS = new Map<string, THREE.MeshBasicMaterial>();
function getPulseMaterial(colorHex: string): THREE.MeshBasicMaterial {
  let mat = PULSE_MATERIALS.get(colorHex);
  if (!mat) {
    mat = new THREE.MeshBasicMaterial({
      color: new THREE.Color(colorHex),
      transparent: true,
      opacity: 0.95
    });
    PULSE_MATERIALS.set(colorHex, mat);
  }
  return mat;
}

function canonicalLinkId(a: string, b: string): string {
  return a <= b ? `${a}<->${b}` : `${b}<->${a}`;
}

export const NeuralGraph3D: React.FC<NeuralGraph3DProps> = ({
  nodes,
  links,
  enabledProtocols,
  searchQuery,
  selectedNodeId,
  selectedLinkId,
  bloomStrength,
  paused,
  focusNodeId,
  resetCameraSeq,
  pulseQueueRef,
  onSelectNode,
  onSelectLink,
  onNodeDragPersist
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const fgRef = useRef<ForceGraphMethods<NodeInfo, LinkInfo> | undefined>(undefined);

  const [dimensions, setDimensions] = useState({ width: window.innerWidth, height: window.innerHeight });
  const [graphData, setGraphData] = useState<{ nodes: NodeInfo[]; links: LinkInfo[] }>({
    nodes: [],
    links: []
  });

  // Referencias vivas para mutar sin reiniciar la simulación física 3D
  const nodeMapRef = useRef<Map<string, NodeInfo>>(new Map());
  const linkMapRef = useRef<Map<string, LinkWithCurve>>(new Map());
  const neuronVisualsRef = useRef<Map<string, NeuronVisual>>(new Map());
  const flashUntilRef = useRef<Map<string, number>>(new Map());
  const bloomPassRef = useRef<UnrealBloomPass | null>(null);
  const pulseLayerRef = useRef<THREE.Group | null>(null);
  const activePulsesRef = useRef<ActivePulse[]>([]);

  const selectedNodeRef = useRef(selectedNodeId);
  selectedNodeRef.current = selectedNodeId;
  const selectedLinkRef = useRef(selectedLinkId);
  selectedLinkRef.current = selectedLinkId;
  const searchQueryRef = useRef(searchQuery);
  searchQueryRef.current = searchQuery;
  const pausedRef = useRef(paused);
  pausedRef.current = paused;

  // Medir tamaño del contenedor
  useEffect(() => {
    const updateSize = () => {
      if (!containerRef.current) return;
      const rect = containerRef.current.getBoundingClientRect();
      const w = Math.max(320, Math.floor(rect.width));
      const h = Math.max(320, Math.floor(rect.height));
      setDimensions({ width: w, height: h });
      if (bloomPassRef.current) {
        bloomPassRef.current.setSize(w, h);
      }
    };
    updateSize();
    window.addEventListener('resize', updateSize);
    return () => window.removeEventListener('resize', updateSize);
  }, []);

  // Sincronizar nodos y enlaces preservando coordenadas x, y, z, vx, vy, vz
  useEffect(() => {
    const nodeMap = nodeMapRef.current;
    const linkMap = linkMapRef.current;
    let topologyChanged = false;

    const incomingIds = new Set<string>();
    for (const n of nodes) {
      incomingIds.add(n.id);
      const existing = nodeMap.get(n.id);
      if (existing) {
        existing.mac = n.mac;
        existing.vendor = n.vendor;
        existing.hostname = n.hostname;
        existing.alias = n.alias;
        existing.category = n.category;
        existing.role = n.role;
        existing.rxBps = n.rxBps;
        existing.txBps = n.txBps;
        existing.rxPps = n.rxPps;
        existing.txPps = n.txPps;
        existing.totalRxBytes = n.totalRxBytes;
        existing.totalTxBytes = n.totalTxBytes;
        existing.totalPackets = n.totalPackets;
        existing.protocols = n.protocols;
        existing.activePorts = n.activePorts;
        existing.activePeers = n.activePeers;
        existing.pinned = n.pinned;
        existing.lastSeen = n.lastSeen;
        existing.energy = n.energy;
        if (n.pinned && n.fx !== undefined && n.fy !== undefined && n.fz !== undefined) {
          existing.fx = n.fx;
          existing.fy = n.fy;
          existing.fz = n.fz;
        } else if (!n.pinned) {
          delete existing.fx;
          delete existing.fy;
          delete existing.fz;
        }
      } else {
        const clone: NodeInfo = { ...n };
        if (n.role === 'host' && !n.pinned) {
          clone.x = 0;
          clone.y = 0;
          clone.z = 0;
        }
        nodeMap.set(n.id, clone);
        topologyChanged = true;
      }
    }

    for (const id of Array.from(nodeMap.keys())) {
      if (!incomingIds.has(id)) {
        nodeMap.delete(id);
        neuronVisualsRef.current.delete(id);
        topologyChanged = true;
      }
    }

    const filteredLinks = links.filter((l) => linkMatchesProtocols(l, enabledProtocols));
    const incomingLinkIds = new Set<string>();

    for (const l of filteredLinks) {
      const sId = endpointId(l.source);
      const tId = endpointId(l.target);
      if (!nodeMap.has(sId) || !nodeMap.has(tId)) continue;

      incomingLinkIds.add(l.id);
      const existing = linkMap.get(l.id);
      if (existing) {
        existing.bps = l.bps;
        existing.pps = l.pps;
        existing.totalBytes = l.totalBytes;
        existing.totalPackets = l.totalPackets;
        existing.dominantProtocol = l.dominantProtocol;
        existing.protocols = l.protocols;
        existing.ports = l.ports;
        existing.lastSeen = l.lastSeen;
      } else {
        linkMap.set(l.id, {
          ...l,
          source: nodeMap.get(sId) || sId,
          target: nodeMap.get(tId) || tId
        });
        topologyChanged = true;
      }
    }

    for (const id of Array.from(linkMap.keys())) {
      if (!incomingLinkIds.has(id)) {
        linkMap.delete(id);
        topologyChanged = true;
      }
    }

    if (topologyChanged) {
      setGraphData({
        nodes: Array.from(nodeMap.values()),
        links: Array.from(linkMap.values())
      });
    }
  }, [nodes, links, enabledProtocols]);

  // Configurar UnrealBloomPass, luz ambiental y capa de pulsos Three.js
  useEffect(() => {
    const fg = fgRef.current;
    if (!fg) return;

    const scene = fg.scene();
    if (!pulseLayerRef.current) {
      const group = new THREE.Group();
      group.name = 'neural-pulses-layer';
      scene.add(group);
      pulseLayerRef.current = group;

      // Iluminación sutil para destacar el volumen 3D de las neuronas
      const ambient = new THREE.AmbientLight(0xdbeafe, 0.85);
      const dir = new THREE.DirectionalLight(0x38bdf8, 1.2);
      dir.position.set(120, 180, 140);
      scene.add(ambient);
      scene.add(dir);
    }

    const composer = fg.postProcessingComposer();
    if (composer && !bloomPassRef.current) {
      const bloomPass = new UnrealBloomPass(
        new THREE.Vector2(dimensions.width, dimensions.height),
        bloomStrength,
        0.55,
        0.16
      );
      composer.addPass(bloomPass);
      bloomPassRef.current = bloomPass;
    }

    // Configurar fuerzas para una distribución neuronal espaciosa
    const chargeForce = fg.d3Force('charge') as { strength?: (v: number) => void } | undefined;
    if (chargeForce?.strength) {
      chargeForce.strength(-220);
    }
    const linkForce = fg.d3Force('link') as { distance?: (fn: () => number) => void } | undefined;
    if (linkForce?.distance) {
      linkForce.distance(() => 85);
    }
  }, [dimensions.width, dimensions.height]);

  useEffect(() => {
    if (bloomPassRef.current) {
      bloomPassRef.current.strength = bloomStrength;
    }
  }, [bloomStrength]);

  // Enfocar cámara en nodo buscado o al reiniciar cámara
  useEffect(() => {
    const fg = fgRef.current;
    if (!fg || !focusNodeId) return;
    const target = nodeMapRef.current.get(focusNodeId);
    if (!target || target.x === undefined || target.y === undefined || target.z === undefined) return;
    const dist = 140;
    const hypot = Math.hypot(target.x, target.y, target.z) || 1;
    const ratio = 1 + dist / hypot;
    fg.cameraPosition(
      { x: target.x * ratio + 30, y: target.y * ratio + 20, z: target.z * ratio + 30 },
      { x: target.x, y: target.y, z: target.z },
      900
    );
  }, [focusNodeId]);

  useEffect(() => {
    const fg = fgRef.current;
    if (!fg || resetCameraSeq === 0) return;
    fg.cameraPosition({ x: 0, y: 80, z: 320 }, { x: 0, y: 0, z: 0 }, 900);
  }, [resetCameraSeq]);

  // Bucle 60 FPS para animación neuronal + trayectorias de pulsos luminosos
  useEffect(() => {
    let rafId = 0;
    let lastTime = performance.now();

    const animate = (now: number) => {
      rafId = requestAnimationFrame(animate);
      const dt = Math.min(0.05, (now - lastTime) / 1000);
      lastTime = now;

      const pulseLayer = pulseLayerRef.current;
      const nodeMap = nodeMapRef.current;
      const linkMap = linkMapRef.current;

      // 1. Drenar nuevos pulsos recibidos por WebSocket
      if (pulseLayer && !pausedRef.current && pulseQueueRef.current.length > 0) {
        const queued = pulseQueueRef.current.splice(0, pulseQueueRef.current.length);
        for (const p of queued) {
          if (activePulsesRef.current.length >= MAX_ACTIVE_PULSES) break;
          const srcNode = nodeMap.get(p.src);
          const dstNode = nodeMap.get(p.dst);
          if (!srcNode || !dstNode) continue;

          const count = Math.max(1, Math.min(5, p.particleCount || 1));
          const colorHex = getProtocolColor(p.protocol);
          const mat = getPulseMaterial(colorHex);
          const linkId = canonicalLinkId(p.src, p.dst);

          for (let i = 0; i < count; i++) {
            if (activePulsesRef.current.length >= MAX_ACTIVE_PULSES) break;
            const mesh = new THREE.Mesh(SHARED_PULSE_GEO, mat);
            const radius = 1.3 + p.intensity * 2.5;
            mesh.scale.setScalar(radius);
            mesh.visible = false;
            pulseLayer.add(mesh);

            activePulsesRef.current.push({
              mesh,
              srcId: p.src,
              dstId: p.dst,
              linkId,
              progress: 0,
              delay: i * 0.055,
              speed: p.speed || 1.0,
              radius
            });
          }
        }
      }

      // 2. Actualizar posiciones de pulsos en vuelo sobre sinapsis
      if (pulseLayer && !pausedRef.current && activePulsesRef.current.length > 0) {
        const remaining: ActivePulse[] = [];
        for (const ap of activePulsesRef.current) {
          if (ap.delay > 0) {
            ap.delay -= dt;
            remaining.push(ap);
            continue;
          }

          const srcNode = nodeMap.get(ap.srcId);
          const dstNode = nodeMap.get(ap.dstId);
          if (!srcNode || !dstNode || srcNode.x === undefined || dstNode.x === undefined) {
            pulseLayer.remove(ap.mesh);
            continue;
          }

          ap.progress += dt * ap.speed * 1.18;
          if (ap.progress >= 1) {
            pulseLayer.remove(ap.mesh);
            flashUntilRef.current.set(ap.dstId, now + 220);
            continue;
          }

          ap.mesh.visible = true;
          const linkObj = linkMap.get(ap.linkId);
          if (linkObj && linkObj.__curve && typeof linkObj.__curve.getPoint === 'function') {
            const linkSrcId = endpointId(linkObj.source);
            const t = linkSrcId === ap.srcId ? ap.progress : 1 - ap.progress;
            const pos = linkObj.__curve.getPoint(Math.max(0, Math.min(1, t)));
            ap.mesh.position.copy(pos);
          } else {
            const t = ap.progress;
            ap.mesh.position.set(
              (srcNode.x ?? 0) + ((dstNode.x ?? 0) - (srcNode.x ?? 0)) * t,
              (srcNode.y ?? 0) + ((dstNode.y ?? 0) - (srcNode.y ?? 0)) * t,
              (srcNode.z ?? 0) + ((dstNode.z ?? 0) - (srcNode.z ?? 0)) * t
            );
          }

          // Suave atenuación en los extremos de la sinapsis
          const envelope = Math.sin(ap.progress * Math.PI);
          ap.mesh.scale.setScalar(ap.radius * (0.65 + 0.55 * envelope));
          remaining.push(ap);
        }
        activePulsesRef.current = remaining;
      }

      // 3. Animar somas neuronales (energía, destello al recibir pulso, selección y búsqueda)
      const q = searchQueryRef.current.trim();
      for (const [id, vis] of neuronVisualsRef.current.entries()) {
        const node = nodeMap.get(id);
        if (!node) continue;

        const isSelected = selectedNodeRef.current === id;
        const isMatched = q !== '' && matchesNodeSearch(node, q);
        const flashUntil = flashUntilRef.current.get(id) ?? 0;
        const flashRatio = flashUntil > now ? (flashUntil - now) / 220 : 0;

        const bps = node.rxBps + node.txBps;
        const trafficScale = Math.min(0.9, Math.log10(1 + bps / 1000) * 0.24);
        const baseScale = node.role === 'host' ? 1.38 : node.role === 'gateway' ? 1.22 : 1.0;
        const scale = baseScale + trafficScale + node.energy * 0.25 + flashRatio * 0.35;

        vis.core.scale.setScalar(scale);
        vis.halo.scale.setScalar(scale * (1.2 + Math.sin(now * 0.004 + scale) * 0.06));

        const targetColor = getRoleColor(node.role);
        if (vis.lastColor !== targetColor) {
          vis.coreMat.color.set(targetColor);
          vis.coreMat.emissive.set(targetColor);
          vis.haloMat.color.set(targetColor);
          vis.lastColor = targetColor;
        }

        vis.coreMat.emissiveIntensity = 0.65 + node.energy * 0.9 + flashRatio * 1.1 + (isSelected ? 0.8 : 0);
        vis.haloMat.opacity = isMatched || isSelected
          ? 0.42
          : 0.11 + node.energy * 0.24 + flashRatio * 0.32;

        vis.ring.visible = Boolean(node.pinned || isSelected || isMatched);
        if (vis.ring.visible) {
          vis.ring.scale.setScalar(scale * 1.05);
          vis.ringMat.color.set(isSelected ? '#ffffff' : isMatched ? '#fbbf24' : '#38bdf8');
          vis.ringMat.opacity = isSelected ? 0.95 : 0.65;
        }

        const displayName = getNodeDisplayName(node);
        const labelStr = bps > 500 ? `${displayName}\n${formatBps(bps)}` : displayName;
        if (vis.lastLabelText !== labelStr) {
          vis.label.text = labelStr;
          vis.lastLabelText = labelStr;
        }
        vis.label.position.set(0, 7.5 * scale + 4.5, 0);
      }
    };

    rafId = requestAnimationFrame(animate);
    return () => cancelAnimationFrame(rafId);
  }, [pulseQueueRef]);

  const buildNeuronObject = (node: NodeInfo): THREE.Object3D => {
    const existing = neuronVisualsRef.current.get(node.id);
    if (existing) {
      return existing.group;
    }

    const group = new THREE.Group();
    const color = getRoleColor(node.role);

    const coreMat = new THREE.MeshStandardMaterial({
      color: new THREE.Color(color),
      emissive: new THREE.Color(color),
      emissiveIntensity: 0.85,
      roughness: 0.2,
      metalness: 0.1
    });
    const core = new THREE.Mesh(SHARED_CORE_GEO, coreMat);

    const haloMat = new THREE.MeshBasicMaterial({
      color: new THREE.Color(color),
      transparent: true,
      opacity: 0.18,
      depthWrite: false
    });
    const halo = new THREE.Mesh(SHARED_HALO_GEO, haloMat);

    const ringMat = new THREE.MeshBasicMaterial({
      color: new THREE.Color('#38bdf8'),
      transparent: true,
      opacity: 0.7
    });
    const ring = new THREE.Mesh(SHARED_RING_GEO, ringMat);
    ring.visible = Boolean(node.pinned);

    const displayName = getNodeDisplayName(node);
    const label = new SpriteText(displayName, 3.3, '#e2e8f0');
    label.fontFace = 'Inter, system-ui, sans-serif';
    label.fontWeight = '600';
    label.backgroundColor = 'rgba(4, 7, 17, 0.62)';
    label.borderColor = 'rgba(56, 189, 248, 0.25)';
    label.borderWidth = 0.25;
    label.borderRadius = 3;
    label.padding = [1.4, 2.6];
    label.position.set(0, 12, 0);

    group.add(halo);
    group.add(core);
    group.add(ring);
    group.add(label);

    neuronVisualsRef.current.set(node.id, {
      group,
      core,
      coreMat,
      halo,
      haloMat,
      ring,
      ringMat,
      label,
      lastLabelText: displayName,
      lastColor: color
    });

    return group;
  };

  return (
    <div ref={containerRef} className="neural-canvas-container">
      <ForceGraph3D
        ref={fgRef}
        width={dimensions.width}
        height={dimensions.height}
        graphData={graphData}
        backgroundColor="#040711"
        showNavInfo={false}
        nodeId="id"
        nodeThreeObject={(n) => buildNeuronObject(n as NodeInfo)}
        nodeThreeObjectExtend={false}
        linkSource="source"
        linkTarget="target"
        linkCurvature={0.22}
        linkWidth={(l) => {
          const link = l as LinkInfo;
          if (selectedLinkId === link.id) return 2.4;
          return 0.5 + Math.min(2.2, Math.log10(1 + link.bps / 1000) * 0.55);
        }}
        linkColor={(l) => {
          const link = l as LinkInfo;
          if (selectedLinkId === link.id) return '#ffffff';
          return getProtocolColor(link.dominantProtocol);
        }}
        linkOpacity={0.34}
        enableNodeDrag={true}
        onNodeClick={(n) => {
          const node = n as NodeInfo;
          onSelectLink(null);
          onSelectNode(node);
        }}
        onLinkClick={(l) => {
          const link = l as LinkInfo;
          onSelectNode(null);
          onSelectLink(link);
        }}
        onBackgroundClick={() => {
          onSelectNode(null);
          onSelectLink(null);
        }}
        onNodeDragEnd={(n) => {
          const node = n as NodeInfo;
          if (node.x !== undefined && node.y !== undefined && node.z !== undefined) {
            node.fx = node.x;
            node.fy = node.y;
            node.fz = node.z;
            onNodeDragPersist(node, node.x, node.y, node.z);
          }
        }}
      />
    </div>
  );
};
