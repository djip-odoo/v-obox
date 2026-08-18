//go:build darwin

package firewall

func activeFirewall() string {
	return "darwin"
}
