package capture

import (
	"bufio"
	"net"
	"os"
	"strconv"
	"strings"
)

// ouiVendors contiene prefijos MAC comunes para identificación local 100% offline.
var ouiVendors = map[string]string{
	"02:42:ac": "Docker Container",
	"00:50:56": "VMware",
	"00:0c:29": "VMware",
	"00:05:69": "VMware",
	"08:00:27": "Oracle VirtualBox",
	"00:15:5d": "Microsoft Hyper-V",
	"52:54:00": "QEMU / KVM",
	"b8:27:eb": "Raspberry Pi Foundation",
	"dc:a6:32": "Raspberry Pi Trading",
	"e4:5f:01": "Raspberry Pi Trading",
	"28:cd:c1": "Raspberry Pi Trading",
	"d8:3a:dd": "Raspberry Pi Trading",
	"2c:cf:67": "Raspberry Pi Trading",
	"fc:ec:da": "Ubiquiti Networks",
	"74:83:c2": "Ubiquiti Networks",
	"b4:fb:e4": "Ubiquiti Networks",
	"24:5a:4c": "Ubiquiti Networks",
	"78:8a:20": "Ubiquiti Networks",
	"68:d7:9a": "Ubiquiti Networks",
	"4c:5e:0c": "MikroTik",
	"cc:2d:e0": "MikroTik",
	"dc:2c:6e": "MikroTik",
	"b8:69:f4": "MikroTik",
	"d4:01:c3": "MikroTik",
	"50:c7:bf": "TP-Link",
	"b0:be:76": "TP-Link",
	"98:da:c4": "TP-Link",
	"c0:06:c3": "TP-Link",
	"14:eb:b6": "TP-Link",
	"f0:18:98": "Apple",
	"3c:22:fb": "Apple",
	"a4:83:e7": "Apple",
	"dc:a9:04": "Apple",
	"00:1c:b3": "Apple",
	"28:cf:e9": "Apple",
	"00:1b:21": "Intel Corporate",
	"3c:fd:fe": "Intel Corporate",
	"a0:36:9f": "Intel Corporate",
	"48:21:0b": "Intel Corporate",
	"8c:8d:28": "Intel Corporate",
	"00:e0:4c": "Realtek Semiconductor",
	"70:85:c2": "ASRock / Realtek",
	"1c:1b:0d": "Gigabyte Technology",
	"00:16:32": "Samsung Electronics",
	"94:35:0a": "Samsung Electronics",
	"c8:ba:94": "Samsung Electronics",
	"f4:7b:5e": "Samsung Electronics",
	"24:0a:c4": "Espressif (IoT)",
	"30:ae:a4": "Espressif (IoT)",
	"7c:9e:bd": "Espressif (IoT)",
	"cc:50:e3": "Espressif (IoT)",
	"ec:fa:bc": "Espressif (IoT)",
	"00:11:32": "Synology",
	"90:09:d0": "Synology",
	"00:14:22": "Dell",
	"18:03:73": "Dell",
	"f8:b1:56": "Dell",
	"3c:d9:2b": "Hewlett Packard",
	"00:25:b3": "Hewlett Packard",
	"94:57:a5": "Hewlett Packard",
	"04:92:26": "ASUSTek Computer",
	"2c:fd:a1": "ASUSTek Computer",
	"ac:9e:17": "ASUSTek Computer",
	"f4:f5:d8": "Google",
	"54:60:09": "Google",
	"3c:5a:b4": "Google",
	"fc:65:de": "Amazon Technologies",
	"44:65:0d": "Amazon Technologies",
	"68:37:e9": "Amazon Technologies",
	"00:1a:11": "Google Nest",
	"74:da:38": "Edimax Technology",
	"00:1e:06": "WIBRAIN / Odroid",
}

// LookupVendor devuelve el fabricante estimado a partir de los 3 primeros octetos de la MAC.
func LookupVendor(mac string) string {
	norm := strings.ToLower(strings.TrimSpace(mac))
	norm = strings.ReplaceAll(norm, "-", ":")
	parts := strings.Split(norm, ":")
	if len(parts) < 3 {
		return ""
	}
	prefix := strings.Join(parts[:3], ":")
	if vendor, ok := ouiVendors[prefix]; ok {
		return vendor
	}
	if strings.HasPrefix(prefix, "02:42:") {
		return "Docker Virtual NIC"
	}
	firstByte, err := strconv.ParseUint(parts[0], 16, 8)
	if err == nil && (firstByte&0x02) != 0 {
		return "Virtual / Privada (LAA)"
	}
	return ""
}

var cgnatNet = func() *net.IPNet {
	_, cidr, _ := net.ParseCIDR("100.64.0.0/10")
	return cidr
}()

// IsPrivateOrLocalIP indica si una IP pertenece a la red local, loopback, multicast o CGNAT.
func IsPrivateOrLocalIP(ipStr string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return true
	}
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4.Equal(net.IPv4bcast) {
			return true
		}
		if cgnatNet != nil && cgnatNet.Contains(ip4) {
			return true
		}
	}
	return false
}

// DetectHostNetwork descubre las IPs locales del host y el gateway por defecto desde /proc/net/route.
func DetectHostNetwork() (map[string]bool, string) {
	hostIPs := make(map[string]bool)
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip != nil && ip.To4() != nil && !ip.IsMulticast() && !ip.IsUnspecified() {
					hostIPs[ip.String()] = true
				}
			}
		}
	}

	gatewayIP := DetectDefaultGateway()
	return hostIPs, gatewayIP
}

// DetectDefaultGateway lee /proc/net/route en Linux para encontrar la IP del gateway por defecto.
func DetectDefaultGateway() string {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Saltar cabecera
	if !scanner.Scan() {
		return ""
	}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		dest := fields[1]
		gwHex := fields[2]
		flagsVal, err := strconv.ParseUint(fields[3], 16, 32)
		if err != nil {
			continue
		}
		// Destino 00000000 y flag RTF_GATEWAY (0x2)
		if dest == "00000000" && (flagsVal&0x2) != 0 {
			gwVal, err := strconv.ParseUint(gwHex, 16, 32)
			if err != nil || gwVal == 0 {
				continue
			}
			ip := net.IPv4(
				byte(gwVal&0xff),
				byte((gwVal>>8)&0xff),
				byte((gwVal>>16)&0xff),
				byte((gwVal>>24)&0xff),
			)
			return ip.String()
		}
	}
	return ""
}
