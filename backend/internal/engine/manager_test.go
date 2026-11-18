package engine

import (
	"testing"
	"time"

	"neuralnetwork/internal/models"
)

func TestAdaptivePulseAggregation(t *testing.T) {
	mgr, err := NewManager(nil, models.AppSettings{
		GroupExternal: false,
		NodeTTLSec:    120,
		BloomStrength: 1.0,
		CaptureMode:   "live",
		Interface:     "eth0",
	}, false, nil)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	now := time.Now()
	// Flujo 1: paquete único (pulso individual discreto)
	mgr.IngestPacket(models.PacketEvent{
		Timestamp: now,
		SrcIP:     "192.168.1.20",
		DstIP:     "192.168.1.1",
		Protocol:  "ICMP",
		Length:    74,
	})

	// Flujo 2: ráfaga alta de 100 paquetes (macro-pulso adaptativo)
	for i := 0; i < 100; i++ {
		mgr.IngestPacket(models.PacketEvent{
			Timestamp: now,
			SrcIP:     "192.168.1.50",
			DstIP:     "192.168.1.10",
			Protocol:  "TCP",
			SrcPort:   48000,
			DstPort:   445,
			Length:    1460,
		})
	}

	msg := mgr.FlushWindow(now.Add(100 * time.Millisecond))
	if len(msg.Pulses) != 2 {
		t.Fatalf("esperaba 2 pulsos consolidados, obtuve %d", len(msg.Pulses))
	}

	var singlePulse, burstPulse *models.PulseEvent
	for i := range msg.Pulses {
		p := &msg.Pulses[i]
		if p.Protocol == "ICMP" {
			singlePulse = p
		} else if p.Protocol == "TCP" {
			burstPulse = p
		}
	}

	if singlePulse == nil || burstPulse == nil {
		t.Fatalf("faltan pulsos esperados en el snapshot: %+v", msg.Pulses)
	}
	if singlePulse.ParticleCount != 1 {
		t.Errorf("pulso individual debería tener particleCount=1, tuvo %d", singlePulse.ParticleCount)
	}
	if burstPulse.Packets != 100 {
		t.Errorf("macro-pulso debería acumular 100 paquetes, tuvo %d", burstPulse.Packets)
	}
	if burstPulse.Intensity <= singlePulse.Intensity {
		t.Errorf("macro-pulso debería tener mayor intensidad (%v) que pulso individual (%v)",
			burstPulse.Intensity, singlePulse.Intensity)
	}
	if burstPulse.Speed <= singlePulse.Speed {
		t.Errorf("macro-pulso debería tener mayor velocidad (%v) que pulso individual (%v)",
			burstPulse.Speed, singlePulse.Speed)
	}
	if burstPulse.ParticleCount < 3 {
		t.Errorf("macro-pulso debería emitir >=3 partículas agrupadas, tuvo %d", burstPulse.ParticleCount)
	}
}

func TestGroupExternalCollapsesPublicIPsOnly(t *testing.T) {
	mgr, err := NewManager(nil, models.AppSettings{
		GroupExternal: true,
		NodeTTLSec:    120,
		BloomStrength: 1.0,
		CaptureMode:   "live",
		Interface:     "eth0",
	}, false, nil)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	now := time.Now()
	mgr.IngestPacket(models.PacketEvent{
		Timestamp: now,
		SrcIP:     "192.168.1.20",
		DstIP:     "1.1.1.1",
		Protocol:  "DNS",
		DstPort:   53,
		Length:    80,
	})
	mgr.IngestPacket(models.PacketEvent{
		Timestamp: now,
		SrcIP:     "192.168.1.20",
		DstIP:     "140.82.112.4",
		Protocol:  "HTTPS",
		DstPort:   443,
		Length:    512,
	})
	mgr.IngestPacket(models.PacketEvent{
		Timestamp: now,
		SrcIP:     "192.168.1.20",
		DstIP:     "192.168.1.10",
		Protocol:  "TCP",
		DstPort:   445,
		Length:    1200,
	})

	msg := mgr.FlushWindow(now.Add(100 * time.Millisecond))

	nodeIDs := make(map[string]bool)
	for _, n := range msg.Nodes {
		nodeIDs[n.ID] = true
	}

	if !nodeIDs["192.168.1.20"] || !nodeIDs["192.168.1.10"] {
		t.Errorf("los nodos privados LAN deben mantenerse separados: %+v", nodeIDs)
	}
	if !nodeIDs[externalGroupID] {
		t.Errorf("las IPs públicas deben agruparse bajo %q: %+v", externalGroupID, nodeIDs)
	}
	if nodeIDs["1.1.1.1"] || nodeIDs["140.82.112.4"] {
		t.Errorf("no deberían existir nodos externos individuales cuando GroupExternal=true: %+v", nodeIDs)
	}
}
