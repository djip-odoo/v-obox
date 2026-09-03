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
  isActive?: boolean;
}

export interface DeviceInfo {
  platform: string;
  model?: string;
  manufacturer?: string;
  brand?: string;
  device?: string;
  product?: string;
  board?: string;
  hardware?: string;

  osVersion: string;
  apiLevel?: number;
  securityPatch?: string;

  webViewPackage?: string;
  webViewVersion?: string;
  userAgent?: string;

  cpuArch: string;
  cpuCores: number;
  supportedAbis?: string[];
  socModel?: string;

  totalRamBytes?: number;
  availableRamBytes?: number;
  usedRamBytes?: number;
  isLowRam?: boolean;

  totalStorageBytes?: number;
  availableStorageBytes?: number;
  usedStorageBytes?: number;

  batteryLevel?: number;
  isCharging?: boolean;
  pluggedSource?: string;

  screenWidth?: number;
  screenHeight?: number;
  screenDensityDpi?: number;
  screenDensity?: number;
  refreshRate?: number;

  networkType?: string;
  localIp?: string;
  port?: number;

  packageName?: string;
  appVersion?: string;
  appVersionCode?: number;
  uptimeMs?: number;

  serverRunning?: boolean;
  hostname?: string;
  goVersion?: string;
  error?: string;
}

