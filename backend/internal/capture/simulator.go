package capture

import (
	"context"
	"math/rand"
	"time"

	"neuralnetwork/internal/models"
)

type simDevice struct {
	ip       string
	mac      string
	hostname string
}

var simTopology = struct {
	host       simDevice
	gateway    simDevice
	dnsLocal   simDevice
	nas        simDevice
	workstation simDevice
	laptop     simDevice
	tv         simDevice
	iot1       simDevice
	iot2       simDevice
	externals  []simDevice
}{
	host:        simDevice{ip: "192.168.1.50", mac: "dc:a6:32:4a:11:08", hostname: "neural-host.lan"},
	gateway:     simDevice{ip: "192.168.1.1", mac: "fc:ec:da:88:20:01", hostname: "gateway.lan"},
	dnsLocal:    simDevice{ip: "192.168.1.2", mac: "b8:27:eb:12:34:56", hostname: "pihole-dns.lan"},
	nas:         simDevice{ip: "192.168.1.10", mac: "00:11:32:7b:90:12", hostname: "synology-nas.lan"},
	workstation: simDevice{ip: "192.168.1.20", mac: "00:1b:21:99:41:02", hostname: "workstation.lan"},
	laptop:      simDevice{ip: "192.168.1.25", mac: "a4:83:e7:33:12:aa", hostname: "macbook-pro.lan"},
	tv:          simDevice{ip: "192.168.1.40", mac: "c8:ba:94:55:21:09", hostname: "smart-tv.lan"},
	iot1:        simDevice{ip: "192.168.1.60", mac: "24:0a:c4:71:88:01", hostname: "sensor-clima.lan"},
	iot2:        simDevice{ip: "192.168.1.61", mac: "30:ae:a4:90:14:22", hostname: "camara-entrada.lan"},
	externals: []simDevice{
		{ip: "1.1.1.1", hostname: "one.one.one.one"},
		{ip: "8.8.8.8", hostname: "dns.google"},
		{ip: "140.82.112.4", hostname: "github.com"},
		{ip: "142.250.190.46", hostname: "google.com"},
		{ip: "104.16.132.229", hostname: "cloudflare.com"},
		{ip: "52.94.236.248", hostname: "s3.amazonaws.com"},
	},
}

// RunSimulation genera tráfico de red sintético realista (pulsos individuales y ráfagas adaptativas).
func RunSimulation(ctx context.Context, out chan<- models.PacketEvent) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	tick := time.NewTicker(80 * time.Millisecond)
	defer tick.Stop()

	step := 0
	emit := func(ev models.PacketEvent) {
		ev.Timestamp = time.Now()
		select {
		case out <- ev:
		default:
		}
	}

	// Emisión inicial inmediata para poblar la red neuronal sin espera.
	emitInitialHandshake(emit)

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			step++
			nowPhase := (step / 40) % 4 // Cambia de patrón de tráfico cada ~3.2s

			// 1. Tráfico discreto continuo: pings ICMP y telemetría IoT
			if step%3 == 0 {
				emit(models.PacketEvent{
					SrcIP: simTopology.workstation.ip, DstIP: simTopology.gateway.ip,
					SrcMAC: simTopology.workstation.mac, DstMAC: simTopology.gateway.mac,
					Protocol: "ICMP", Length: 74, Info: "Echo Request",
				})
			}
			if step%3 == 1 {
				emit(models.PacketEvent{
					SrcIP: simTopology.gateway.ip, DstIP: simTopology.workstation.ip,
					SrcMAC: simTopology.gateway.mac, DstMAC: simTopology.workstation.mac,
					Protocol: "ICMP", Length: 74, Info: "Echo Reply",
				})
			}

			// 2. Consultas DNS puntuales (ámbar)
			if step%4 == 0 {
				ext := simTopology.externals[rng.Intn(len(simTopology.externals))]
				client := simTopology.laptop
				if rng.Intn(2) == 0 {
					client = simTopology.workstation
				}
				emit(models.PacketEvent{
					SrcIP: client.ip, DstIP: simTopology.dnsLocal.ip,
					SrcMAC: client.mac, DstMAC: simTopology.dnsLocal.mac,
					Protocol: "DNS", SrcPort: uint16(49152 + rng.Intn(10000)), DstPort: 53,
					Length: 82, DNSName: ext.hostname, Info: "DNS Query " + ext.hostname,
				})
				emit(models.PacketEvent{
					SrcIP: simTopology.dnsLocal.ip, DstIP: ext.ip,
					SrcMAC: simTopology.dnsLocal.mac, DstMAC: simTopology.gateway.mac,
					Protocol: "DNS", SrcPort: uint16(40000 + rng.Intn(10000)), DstPort: 53,
					Length: 96, DNSName: ext.hostname, ResolvedIP: ext.ip,
				})
			}

			// 3. Descubrimiento ARP ocasional (naranja)
			if step%12 == 0 {
				iot := simTopology.iot1
				if rng.Intn(2) == 0 {
					iot = simTopology.iot2
				}
				emit(models.PacketEvent{
					SrcIP: simTopology.gateway.ip, DstIP: iot.ip,
					SrcMAC: simTopology.gateway.mac, DstMAC: iot.mac,
					Protocol: "ARP", Length: 42, Info: "ARP Who-has",
				})
			}

			// 4. Telemetría IoT TCP/HTTP hacia servidor host
			if step%5 == 0 {
				emit(models.PacketEvent{
					SrcIP: simTopology.iot1.ip, DstIP: simTopology.host.ip,
					SrcMAC: simTopology.iot1.mac, DstMAC: simTopology.host.mac,
					Protocol: "HTTP", SrcPort: 51200, DstPort: 80,
					Length: 240,
				})
			}
			if step%2 == 0 {
				// Cámara enviando stream UDP liviano al NAS
				for i := 0; i < 3; i++ {
					emit(models.PacketEvent{
						SrcIP: simTopology.iot2.ip, DstIP: simTopology.nas.ip,
						SrcMAC: simTopology.iot2.mac, DstMAC: simTopology.nas.mac,
						Protocol: "UDP", SrcPort: 554, DstPort: 45120,
						Length: 1320,
					})
				}
			}

			// 5. Navegación HTTPS y APIs hacia Internet
			if step%2 == 0 {
				ext := simTopology.externals[2+rng.Intn(len(simTopology.externals)-2)]
				src := simTopology.workstation
				if rng.Intn(3) == 0 {
					src = simTopology.host
				}
				emit(models.PacketEvent{
					SrcIP: src.ip, DstIP: ext.ip,
					SrcMAC: src.mac, DstMAC: simTopology.gateway.mac,
					Protocol: "HTTPS", SrcPort: 54210, DstPort: 443,
					Length: 512, ResolvedIP: ext.ip, DNSName: ext.hostname,
				})
				emit(models.PacketEvent{
					SrcIP: ext.ip, DstIP: src.ip,
					SrcMAC: simTopology.gateway.mac, DstMAC: src.mac,
					Protocol: "HTTPS", SrcPort: 443, DstPort: 54210,
					Length: 1420, ResolvedIP: ext.ip, DNSName: ext.hostname,
				})
			}

			// 6. Ráfagas de alto caudal (demostración de agrupación adaptativa de macro-pulsos)
			switch nowPhase {
			case 0, 2:
				// Backup TCP intenso de Servidor Host -> Synology NAS
				burst := 14 + rng.Intn(20)
				for i := 0; i < burst; i++ {
					emit(models.PacketEvent{
						SrcIP: simTopology.host.ip, DstIP: simTopology.nas.ip,
						SrcMAC: simTopology.host.mac, DstMAC: simTopology.nas.mac,
						Protocol: "TCP", SrcPort: 48122, DstPort: 445,
						Length: 1460,
					})
				}
			case 1:
				// Streaming 4K UDP/HTTPS desde Gateway/Internet hacia Smart TV y Laptop
				burst := 12 + rng.Intn(16)
				ext := simTopology.externals[4]
				for i := 0; i < burst; i++ {
					emit(models.PacketEvent{
						SrcIP: ext.ip, DstIP: simTopology.tv.ip,
						SrcMAC: simTopology.gateway.mac, DstMAC: simTopology.tv.mac,
						Protocol: "UDP", SrcPort: 443, DstPort: 51920,
						Length: 1380, ResolvedIP: ext.ip, DNSName: ext.hostname,
					})
				}
			case 3:
				// Descarga HTTPS desde GitHub/Cloud hacia Workstation
				burst := 10 + rng.Intn(18)
				ext := simTopology.externals[2]
				for i := 0; i < burst; i++ {
					emit(models.PacketEvent{
						SrcIP: ext.ip, DstIP: simTopology.workstation.ip,
						SrcMAC: simTopology.gateway.mac, DstMAC: simTopology.workstation.mac,
						Protocol: "HTTPS", SrcPort: 443, DstPort: 59012,
						Length: 1480, ResolvedIP: ext.ip, DNSName: ext.hostname,
					})
				}
			}
		}
	}
}

func emitInitialHandshake(emit func(models.PacketEvent)) {
	initial := []models.PacketEvent{
		{SrcIP: simTopology.host.ip, DstIP: simTopology.gateway.ip, SrcMAC: simTopology.host.mac, DstMAC: simTopology.gateway.mac, Protocol: "ARP", Length: 42},
		{SrcIP: simTopology.workstation.ip, DstIP: simTopology.host.ip, SrcMAC: simTopology.workstation.mac, DstMAC: simTopology.host.mac, Protocol: "TCP", SrcPort: 52011, DstPort: 22, Length: 180},
		{SrcIP: simTopology.laptop.ip, DstIP: simTopology.dnsLocal.ip, SrcMAC: simTopology.laptop.mac, DstMAC: simTopology.dnsLocal.mac, Protocol: "DNS", SrcPort: 53012, DstPort: 53, Length: 84, DNSName: "github.com"},
		{SrcIP: simTopology.host.ip, DstIP: simTopology.nas.ip, SrcMAC: simTopology.host.mac, DstMAC: simTopology.nas.mac, Protocol: "TCP", SrcPort: 48122, DstPort: 445, Length: 1200},
		{SrcIP: simTopology.iot1.ip, DstIP: simTopology.host.ip, SrcMAC: simTopology.iot1.mac, DstMAC: simTopology.host.mac, Protocol: "HTTP", SrcPort: 50110, DstPort: 80, Length: 220},
		{SrcIP: simTopology.gateway.ip, DstIP: simTopology.externals[0].ip, SrcMAC: simTopology.gateway.mac, Protocol: "DNS", SrcPort: 51200, DstPort: 53, Length: 92, ResolvedIP: simTopology.externals[0].ip, DNSName: simTopology.externals[0].hostname},
	}
	for _, ev := range initial {
		emit(ev)
	}
}

// SimulatedHostSet devuelve las IPs simuladas de Host y Gateway para etiquetado visual correcto en modo demo.
func SimulatedHostSet() (map[string]bool, string, map[string]string) {
	hosts := map[string]bool{
		simTopology.host.ip: true,
	}
	names := map[string]string{
		simTopology.host.ip:        simTopology.host.hostname,
		simTopology.gateway.ip:     simTopology.gateway.hostname,
		simTopology.dnsLocal.ip:    simTopology.dnsLocal.hostname,
		simTopology.nas.ip:         simTopology.nas.hostname,
		simTopology.workstation.ip: simTopology.workstation.hostname,
		simTopology.laptop.ip:      simTopology.laptop.hostname,
		simTopology.tv.ip:          simTopology.tv.hostname,
		simTopology.iot1.ip:        simTopology.iot1.hostname,
		simTopology.iot2.ip:        simTopology.iot2.hostname,
	}
	for _, ext := range simTopology.externals {
		names[ext.ip] = ext.hostname
	}
	return hosts, simTopology.gateway.ip, names
}
