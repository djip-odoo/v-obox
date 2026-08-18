import type { main } from "../../../wailsjs/go/models";

export function windowsSteps(port: number, execPath: string) {
  const programPath = execPath || "C:\\Program Files\\ePOS Proxy\\epos-proxy.exe";
  return [
    {
      title: "Windows Firewall Rule",
      desc: "If Windows Defender Firewall is blocking other devices on your local network, open *PowerShell as Administrator* and run:",
      codes: [
        `New-NetFirewallRule -DisplayName "ePOS Proxy" -Direction Inbound \`\n  -Program "${programPath}" \`\n  -Action Allow -Profile Private`,
      ],
    },
    ...networkSteps(port),
  ];
}

export function macSteps(port: number, localIp: string) {
  return [
    {
      title: "macOS Application Firewall",
      desc: "macOS uses an application firewall. You can allow ePOS Proxy in *System Settings → Privacy & Security → Firewall*, or run this in Terminal:",
      codes: [
        `sudo /usr/libexec/ApplicationFirewall/socketfilterfw --unblockapp "/Applications/ePOS Proxy.app"`,
      ],
    },
    ...networkSteps(port, localIp),
  ];
}

export function linuxFirewalldSteps(port: number, subnet: string, localIp: string) {
  return [
    {
      title: "Allow Port in Firewalld",
      desc: `*firewalld* is active and blocks inbound traffic by default. Run the following commands in your terminal:`,
      codes: [
        `sudo firewall-cmd --permanent --zone=home --add-port=${port}/tcp\nsudo firewall-cmd --reload`,
      ],
    },
    ...networkSteps(port, localIp, subnet),
  ];
}

export function linuxUfwSteps(port: number, subnet: string, localIp: string) {
  return [
    {
      title: "Allow Port in UFW",
      desc: `*ufw* is active on your system. Run this command in your terminal to allow printing from your local network (*${subnet}*):`,
      codes: [
        `sudo ufw allow from ${subnet} to any port ${port} proto tcp`,
      ],
    },
    ...networkSteps(port, localIp, subnet),
  ];
}

export function linuxNftablesSteps(port: number, subnet: string, localIp: string) {
  return [
    {
      title: "Allow Port in nftables",
      desc: `*nftables* is active. Add a rule to allow incoming TCP traffic on port *${port}* from your local network (*${subnet}*).\n\n⚠️ This rule is not persistent across reboots. Save it to your nftables config (e.g. \`/etc/nftables.conf\`) to make it permanent.`,
      codes: [
        `sudo nft add rule inet filter input ip saddr ${subnet} tcp dport ${port} accept`,
      ],
    },
    ...networkSteps(port, localIp, subnet),
  ];
}

export function linuxNoFirewallSteps(port: number, localIp: string) {
  return [
    {
      title: "Linux Firewall Rule",
      desc: `Install any firewall package like ufw, firewalld or nftables and allow port ${port} for incoming connections. Then open this dialog again.`,
    },
    ...networkSteps(port, localIp),
  ];
}

function networkSteps(port: number, localIp?: string, subnet?: string) {
  const ip = localIp || "127.0.0.1";
  return [
    {
      title: "Check Network & Wi-Fi Connection",
      desc: `1. *Same Local Wi-Fi:* Ensure your POS tablet or device is connected to the same Wi-Fi network (not a Guest network or cellular data).\n\n2. *Router Client Isolation:* Check if your Wi-Fi router has "Client Isolation" or "AP Isolation" enabled. This prevents devices from communicating with each other.\n\n3. *Proxy Server Address:* Your proxy server is listening at *http://${ip}:${port}*`,
    },
    {
      title: "Set a Fixed / Static IP",
      desc: `To ensure your POS connection never breaks when your router reboots or assigns new IP addresses:\n\n1. *Reserve a fixed / static IP* for this computer (*${ip}*) in your router's DHCP settings.\n\n2. *Enter this fixed address* in your Odoo POS IoT / ePOS configuration:\n*http://${ip}:${port}*`,
    },
  ];
}

export function getTroubleshootSteps(info: main.TroubleshootInfo) {
  const port = info.port || 8080;
  const subnet = info.subnet || "192.168.1.0/24";
  const localIp = info.localIp || "127.0.0.1";
  const execPath = info.execPath || "";

  if (info.os === "windows") {
    return windowsSteps(port, execPath);
  }
  if (info.os === "darwin") {
    return macSteps(port, localIp);
  }
  if (info.os === "linux") {
    if (info.activeFirewall === "firewalld") {
      return linuxFirewalldSteps(port, subnet, localIp);
    }
    if (info.activeFirewall === "ufw") {
      return linuxUfwSteps(port, subnet, localIp);
    }
    if (info.activeFirewall === "nftables") {
      return linuxNftablesSteps(port, subnet, localIp);
    }
    return linuxNoFirewallSteps(port, localIp);
  }
  return networkSteps(port, localIp, subnet);
}
