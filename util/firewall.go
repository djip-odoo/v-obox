package util

import "errors"

var ErrAuthCancelled = errors.New("authentication cancelled")

func SetFirewallRule(port, oldPort int) error {
	return setFirewallRule(port, oldPort)
}

func UnsetFirewallRule(port int) error {
	return unsetFirewallRule(port)
}
