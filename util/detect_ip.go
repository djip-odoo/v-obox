package util

import (
	"epos-proxy/logger"
	"net"
)

func GetLocalIP(isFirewallEnabled bool) string {
	if !isFirewallEnabled {
		return "127.0.0.1"
	}

	logger.Debugf("Detecting local LAN IP address...")
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()

		logger.Debugf("UDP dial successful, checking local address...")
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			if ip := addr.IP.To4(); ip != nil {
				logger.Debugf("Detected LAN IP via UDP route: %s", ip.String())
				return ip.String()
			}

			logger.Warnf("UDP local address is not IPv4: %v", addr.IP)
		} else {
			logger.Warnf("Failed to cast UDP local address")
		}
	}
	logger.Warnf("UDP dial failed, falling back to localhost ip: %v", err)
	return "127.0.0.1"
}
