package capture

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"

	"neuralnetwork/internal/models"
)

// ListInterfaces enumera las interfaces disponibles para libpcap con sus direcciones IP.
func ListInterfaces() ([]models.NetworkInterface, error) {
	devs, err := pcap.FindAllDevs()
	if err != nil {
		return nil, err
	}

	result := make([]models.NetworkInterface, 0, len(devs))
	for _, dev := range devs {
		if strings.HasPrefix(dev.Name, "nflog") ||
			strings.HasPrefix(dev.Name, "nfqueue") ||
			strings.HasPrefix(dev.Name, "dbus") ||
			strings.HasPrefix(dev.Name, "usbmon") ||
			strings.HasPrefix(dev.Name, "bluetooth") {
			continue
		}

		addrs := make([]string, 0, len(dev.Addresses))
		isLoop := dev.Name == "lo" || dev.Name == "lo0"
		for _, a := range dev.Addresses {
			if a.IP == nil {
				continue
			}
			if a.IP.IsLoopback() {
				isLoop = true
			}
			addrs = append(addrs, a.IP.String())
		}

		desc := dev.Description
		if desc == "" {
			if dev.Name == "any" {
				desc = "Todas las interfaces (Linux cooked)"
			} else if isLoop {
				desc = "Interfaz Loopback"
			} else {
				desc = "Interfaz de red del host"
			}
		}

		result = append(result, models.NetworkInterface{
			Name:        dev.Name,
			Description: desc,
			Addresses:   addrs,
			IsLoopback:  isLoop,
		})
	}
	return result, nil
}

// SelectDefaultInterface elige la mejor interfaz inicial cuando se configura "auto".
func SelectDefaultInterface(preferred string, ifaces []models.NetworkInterface) string {
	if preferred != "" && preferred != "auto" {
		for _, iface := range ifaces {
			if iface.Name == preferred {
				return preferred
			}
		}
		return preferred
	}

	// Priorizar interfaces estándar físicas/principales (eth, ens, enp, wlan, wlp) con dirección IPv4.
	preferredPrefixes := []string{"eth", "ens", "enp", "wlan", "wlp"}
	for _, prefix := range preferredPrefixes {
		for _, iface := range ifaces {
			if iface.IsLoopback || !strings.HasPrefix(iface.Name, prefix) {
				continue
			}
			for _, addr := range iface.Addresses {
				if ip := net.ParseIP(addr); ip != nil && ip.To4() != nil {
					return iface.Name
				}
			}
		}
	}

	// Luego otras interfaces reales no loopback ni docker0 con dirección IPv4.
	for _, iface := range ifaces {
		if iface.IsLoopback || iface.Name == "any" || strings.HasPrefix(iface.Name, "docker") || strings.HasPrefix(iface.Name, "veth") {
			continue
		}
		for _, addr := range iface.Addresses {
			if ip := net.ParseIP(addr); ip != nil && ip.To4() != nil {
				return iface.Name
			}
		}
	}
	// Fallback a eth0 o cualquier interfaz activa
	for _, iface := range ifaces {
		if iface.Name == "eth0" {
			return "eth0"
		}
	}
	for _, iface := range ifaces {
		if !iface.IsLoopback && iface.Name != "any" {
			return iface.Name
		}
	}
	if len(ifaces) > 0 {
		return ifaces[0].Name
	}
	return "eth0"
}

// RunLiveCapture abre la interfaz seleccionada con libpcap y emite PacketEvents hasta cancelar ctx.
func RunLiveCapture(ctx context.Context, ifaceName string, promisc bool, excludedPorts []int, out chan<- models.PacketEvent) error {
	// Snaplen de 256 bytes: suficiente para L2/L3/L4 y preguntas/respuestas DNS sin leer payloads de aplicación.
	handle, err := pcap.OpenLive(ifaceName, 256, promisc, 50*time.Millisecond)
	if err != nil {
		return fmt.Errorf("no se pudo abrir la interfaz %q: %w", ifaceName, err)
	}
	defer handle.Close()

	// Excluir el tráfico propio de la interfaz web/WebSocket para evitar bucles de pulsos.
	if len(excludedPorts) > 0 {
		parts := make([]string, 0, len(excludedPorts))
		for _, p := range excludedPorts {
			if p > 0 {
				parts = append(parts, fmt.Sprintf("tcp port %d", p))
			}
		}
		if len(parts) > 0 {
			bpf := "not (" + strings.Join(parts, " or ") + ")"
			if err := handle.SetBPFFilter(bpf); err != nil {
				log.Printf("[sniffer] advertencia al aplicar filtro BPF %q: %v", bpf, err)
			}
		}
	}

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packets := packetSource.Packets()

	for {
		select {
		case <-ctx.Done():
			return nil
		case pkt, ok := <-packets:
			if !ok {
				return fmt.Errorf("el flujo pcap en %s finalizó inesperadamente", ifaceName)
			}
			if pkt == nil {
				continue
			}
			if ev, valid := decodePacket(pkt); valid {
				select {
				case out <- ev:
				default:
					// Descartar bajo saturación extrema para proteger memoria y latencia en tiempo real.
				}
			}
		}
	}
}

func decodePacket(pkt gopacket.Packet) (models.PacketEvent, bool) {
	ev := models.PacketEvent{
		Timestamp: pkt.Metadata().Timestamp,
		Length:    pkt.Metadata().Length,
		Protocol:  "OTHER",
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	if ev.Length <= 0 {
		ev.Length = len(pkt.Data())
	}

	if ethLayer := pkt.Layer(layers.LayerTypeEthernet); ethLayer != nil {
		eth := ethLayer.(*layers.Ethernet)
		ev.SrcMAC = eth.SrcMAC.String()
		ev.DstMAC = eth.DstMAC.String()
	}

	// Caso ARP
	if arpLayer := pkt.Layer(layers.LayerTypeARP); arpLayer != nil {
		arp := arpLayer.(*layers.ARP)
		ev.Protocol = "ARP"
		ev.SrcIP = net.IP(arp.SourceProtAddress).String()
		ev.DstIP = net.IP(arp.DstProtAddress).String()
		if len(arp.SourceHwAddress) == 6 {
			ev.SrcMAC = net.HardwareAddr(arp.SourceHwAddress).String()
		}
		if len(arp.DstHwAddress) == 6 && ev.DstMAC == "" {
			ev.DstMAC = net.HardwareAddr(arp.DstHwAddress).String()
		}
		if arp.Operation == layers.ARPRequest {
			ev.Info = "ARP Request"
		} else {
			ev.Info = "ARP Reply"
		}
		return ev, isValidEndpointPair(ev.SrcIP, ev.DstIP)
	}

	// Capa IP (v4 o v6)
	if ip4Layer := pkt.Layer(layers.LayerTypeIPv4); ip4Layer != nil {
		ip4 := ip4Layer.(*layers.IPv4)
		ev.SrcIP = ip4.SrcIP.String()
		ev.DstIP = ip4.DstIP.String()
	} else if ip6Layer := pkt.Layer(layers.LayerTypeIPv6); ip6Layer != nil {
		ip6 := ip6Layer.(*layers.IPv6)
		ev.SrcIP = ip6.SrcIP.String()
		ev.DstIP = ip6.DstIP.String()
	} else {
		return ev, false
	}

	if !isValidEndpointPair(ev.SrcIP, ev.DstIP) {
		return ev, false
	}

	// Capa Transporte / Control
	if pkt.Layer(layers.LayerTypeICMPv4) != nil || pkt.Layer(layers.LayerTypeICMPv6) != nil {
		ev.Protocol = "ICMP"
		return ev, true
	}

	if tcpLayer := pkt.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		tcp := tcpLayer.(*layers.TCP)
		ev.SrcPort = uint16(tcp.SrcPort)
		ev.DstPort = uint16(tcp.DstPort)
		ev.Protocol = classifyTCP(ev.SrcPort, ev.DstPort)
	} else if udpLayer := pkt.Layer(layers.LayerTypeUDP); udpLayer != nil {
		udp := udpLayer.(*layers.UDP)
		ev.SrcPort = uint16(udp.SrcPort)
		ev.DstPort = uint16(udp.DstPort)
		ev.Protocol = classifyUDP(ev.SrcPort, ev.DstPort)
	}

	// Capa DNS (pregunta o respuesta para enriquecer nombres pasivos)
	if dnsLayer := pkt.Layer(layers.LayerTypeDNS); dnsLayer != nil {
		dns := dnsLayer.(*layers.DNS)
		ev.Protocol = "DNS"
		if len(dns.Questions) > 0 {
			name := strings.TrimSuffix(string(dns.Questions[0].Name), ".")
			ev.DNSName = name
			ev.Info = "DNS " + name
		}
		if dns.QR && len(dns.Answers) > 0 {
			for _, ans := range dns.Answers {
				if ans.IP != nil {
					ev.ResolvedIP = ans.IP.String()
					if name := strings.TrimSuffix(string(ans.Name), "."); name != "" {
						ev.DNSName = name
					}
					break
				}
			}
		}
	}

	return ev, true
}

func isValidEndpointPair(src, dst string) bool {
	if src == "" || dst == "" || src == "0.0.0.0" || dst == "0.0.0.0" {
		return false
	}
	if dst == "255.255.255.255" {
		return false
	}
	return true
}

func classifyTCP(srcPort, dstPort uint16) string {
	if srcPort == 53 || dstPort == 53 || srcPort == 853 || dstPort == 853 {
		return "DNS"
	}
	if srcPort == 443 || dstPort == 443 || srcPort == 8443 || dstPort == 8443 {
		return "HTTPS"
	}
	if srcPort == 80 || dstPort == 80 || srcPort == 8080 || dstPort == 8080 || srcPort == 3000 || dstPort == 3000 {
		return "HTTP"
	}
	return "TCP"
}

func classifyUDP(srcPort, dstPort uint16) string {
	if srcPort == 53 || dstPort == 53 || srcPort == 5353 || dstPort == 5353 {
		return "DNS"
	}
	if srcPort == 443 || dstPort == 443 { // QUIC / HTTP3
		return "HTTPS"
	}
	return "UDP"
}
