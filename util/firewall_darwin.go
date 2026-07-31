package util

// a no-op on macOS: the OS uses an application-level firewall,
// not port-based rules. The application must be allowed in
// System Settings → Privacy & Security → Firewall.
// The UI surfaces this guidance to the user when running on macOS.
func setFirewallRule(port, oldPort int) error {
	return nil
}

func unsetFirewallRule(port int) error {
	return nil
}
