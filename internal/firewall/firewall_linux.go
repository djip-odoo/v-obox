//go:build linux

package firewall

import "os/exec"

func activeFirewall() string {
	isActive := func(unit string) bool {
		return exec.Command("systemctl", "is-active", "--quiet", unit).Run() == nil
	}
	switch {
	case isActive("firewalld"):
		return "firewalld"
	case isActive("ufw"):
		return "ufw"
	case isActive("nftables"):
		return "nftables"
	}
	return ""
}
