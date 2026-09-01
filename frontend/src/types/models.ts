export interface AppVariable {
  serverRunning: boolean;
  os: string;
}

export interface Printer {
  name: string;
  ip: string;
  id: string;
  isLAN: boolean;
  lanIp?: string;
  online: boolean;
  type: string;
}

export interface UnavailablePrinter {
  name: string;
  errorMsg: string;
  isLAN: boolean;
  lanIp?: string;
}

export interface Printers {
  errorMsg: string;
  printers: Printer[];
  unavailablePrinters: UnavailablePrinter[];
}

export interface TroubleshootInfo {
  activeFirewall: string;
  firewallZone: string;
  port: number;
  subnet: string;
  localIp: string;
  execPath: string;
}

export interface WebViewConfig {
  url: string;
  enabled: boolean;
  hasPIN: boolean;
  zoom?: number;
}
