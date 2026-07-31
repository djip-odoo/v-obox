package util

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"epos-proxy/logger"
)

const firewallRuleName = "EPOS Proxy LAN Access"

func setFirewallRule(port, oldPort int) error {
	return allowApplicationOS()
}

func unsetFirewallRule(port int) error {
	return removeApplicationOS()
}

func removeApplicationOS() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	logger.Infof("Removing Windows firewall rule for %s", exePath)
	deleteCmd := fmt.Sprintf(`netsh advfirewall firewall delete rule name=all program="%s"`, exePath)
	if err := runElevated("cmd.exe", "/c", deleteCmd); err != nil {
		return fmt.Errorf("failed to remove firewall rule: %w", err)
	}
	return nil
}

func allowApplicationOS() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	logger.Infof("Configuring Windows firewall for %s", exePath)
	deleteCmd := fmt.Sprintf(`netsh advfirewall firewall delete rule name=all program="%s"`, exePath)
	addCmd := fmt.Sprintf(`netsh advfirewall firewall add rule name="%s" dir=in action=allow program="%s" enable=yes profile=any`, firewallRuleName, exePath)
	script := deleteCmd + " & " + addCmd
	if err := runElevated("cmd.exe", "/c", script); err != nil {
		return fmt.Errorf("failed to configure firewall rule: %w", err)
	}
	return nil
}

func runElevated(exe string, args ...string) error {
	logger.Infof("Running elevated command: %s %s", exe, strings.Join(args, " "))

	psArgs := make([]string, len(args))
	for i, arg := range args {
		psArgs[i] = "'" + escapePS(arg) + "'"
	}

	psCmd := fmt.Sprintf(`$proc = Start-Process -FilePath '%s' -ArgumentList @(%s) -Verb RunAs -Wait -PassThru; exit $proc.ExitCode`, escapePS(exe), strings.Join(psArgs, ","))
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", psCmd)
	output, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(output))
	if err != nil {
		if strings.Contains(out, "1223") ||
			strings.Contains(strings.ToLower(out), "the operation was canceled by the user") {
			return ErrAuthCancelled
		}
		return fmt.Errorf("failed to execute elevated command: %w\n%s", err, out)
	}
	if out != "" && out != "0" {
		return fmt.Errorf("elevated process exited with code %s", out)
	}
	return nil
}

func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
