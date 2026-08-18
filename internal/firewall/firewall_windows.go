//go:build windows

package firewall

func activeFirewall() string {
	return "windows"
}
