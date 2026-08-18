package util

import (
	"fmt"
	"net"

	"epos-proxy/internal/logger"
)

// getLocalIPv4 dials a UDP route to discover the machine's outbound IPv4 address.
// Returns nil if the address cannot be determined.
func getLocalIPv4() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil
	}
	defer conn.Close()

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return nil
	}
	return addr.IP.To4()
}

func localAddrInfo() (ip net.IP, ipNet *net.IPNet, ifaceName string) {
	ip = getLocalIPv4()
	if ip == nil {
		return nil, nil, ""
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ip, nil, ""
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			n, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			if n.IP.To4() != nil && n.IP.To4().Equal(ip) {
				return ip, n, iface.Name
			}
		}
	}
	return ip, nil, ""
}

func GetLocalIP(isNetworkEnabled bool) string {
	if !isNetworkEnabled {
		return "127.0.0.1"
	}

	logger.Debugf("Detecting local LAN IP address...")
	ip := getLocalIPv4()
	if ip == nil {
		logger.Warnf("UDP dial failed or returned non-IPv4 address, falling back to localhost")
		return "127.0.0.1"
	}

	logger.Debugf("Detected LAN IP via UDP route: %s", ip.String())
	return ip.String()
}

func GetLocalSubnet() string {
	ip, ipNet, _ := localAddrInfo()
	if ip == nil {
		return ""
	}
	if ipNet != nil {
		ones, _ := ipNet.Mask.Size()
		network := ip.Mask(ipNet.Mask)
		return fmt.Sprintf("%s/%d", network, ones)
	}

	return fmt.Sprintf("%d.%d.%d.0/24", ip[0], ip[1], ip[2])
}

func GetLocalInterface() string {
	_, _, name := localAddrInfo()
	return name
}
