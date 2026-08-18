package firewall

import (
	"fmt"
	"runtime"
	"testing"

	"epos-proxy/internal/testutil"
)

var knownLinuxFirewalls = map[string]bool{
	"firewalld": true,
	"ufw":       true,
	"nftables":  true,
	"":          true,
}

func TestActiveFirewall(t *testing.T) {
	fw := ActiveFirewall()

	switch runtime.GOOS {
	case "linux":
		testutil.ExpectedTrue(t, knownLinuxFirewalls[fw],
			fmt.Sprintf("expected one of {firewalld, ufw, nftables, \"\"}, got %q", fw))
	default:
		testutil.ExpectedEqual(t, fw, runtime.GOOS)
	}
}
