package util

import (
	"fmt"
	"os/exec"
	"strings"

	"epos-proxy/logger"
)

func setFirewallRule(port, oldPort int) error {
	return allowPortOS(port, oldPort)
}

func allowPortOS(port, oldPort int) error {
	logger.Infof("Updating UFW rules: old=%d new=%d", oldPort, port)
	if _, err := exec.LookPath("ufw"); err != nil {
		return fmt.Errorf("UFW is not installed. Install UFW or configure the firewall manually")
	}

	var commands []string
	if oldPort > 0 && oldPort != port {
		commands = append(commands,
			fmt.Sprintf("ufw delete allow %d/tcp || true", oldPort),
		)
	}

	commands = append(commands,
		fmt.Sprintf("ufw allow %d/tcp", port),
	)

	return runElevatedLinux(commands)
}

func unsetFirewallRule(port int) error {
	logger.Infof("Disabling port %d", port)
	commands := []string{
		fmt.Sprintf("ufw delete allow %d/tcp", port),
	}

	return runElevatedLinux(commands)
}

func runElevatedLinux(commands []string) error {
	if _, err := exec.LookPath("pkexec"); err == nil {
		return runWithPkexec(commands)
	}

	if _, err := exec.LookPath("kdesudo"); err == nil {
		return runWithKdesudo(commands)
	}

	return fmt.Errorf("no authorization agent found (pkexec or kdesudo)")
}

func runElevatedLinuxCommand(sudoCmd string, prefixArgs []string, commands []string) error {
	script := strings.Join(commands, " ; ")
	logger.Infof("Executing UFW commands with %s: %s", sudoCmd, script)

	args := append(prefixArgs, "sh", "-c", script)
	cmd := exec.Command(sudoCmd, args...)

	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	outStr := strings.ToLower(string(output))

	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode := exitErr.ExitCode()
		if exitCode == 126 {
			return ErrAuthCancelled
		}
		if sudoCmd == "pkexec" && exitCode == 127 {
			return fmt.Errorf("pkexec failed to execute shell")
		}
	}

	if strings.Contains(outStr, "authentication") ||
		strings.Contains(outStr, "cancel") ||
		strings.Contains(outStr, "dismissed") {
		return ErrAuthCancelled
	}

	return fmt.Errorf("%s failed: %w\n%s", sudoCmd, err, output)
}

func runWithPkexec(commands []string) error {
	return runElevatedLinuxCommand("pkexec", nil, commands)
}

func runWithKdesudo(commands []string) error {
	return runElevatedLinuxCommand("kdesudo", []string{"--"}, commands)
}
