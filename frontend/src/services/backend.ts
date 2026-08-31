import { detectWails } from "../contexts/AppContext";
import {
  AddLANPrinter,
  AppVariable,
  CheckLANPrinterStatus,
  ConfirmRemoveLANPrinter,
  DisableAutostart,
  EnableAutostart,
  GetTroubleshootInfo,
  GetWebViewConfig,
  IsAutostartEnabled,
  IsNetworkPrintingEnabled,
  Printers,
  SetNetworkPrintingEnabled,
  SetWebViewEnabled,
  SetWebViewPIN,
  SetWebViewURL,
  SetWindowFullscreen,
  ValidateWebViewPIN,
} from "../../wailsjs/go/main/App";
import {
  apiAddLANPrinter,
  apiCashDrawer,
  apiCreatePINSession,
  apiGetAppVariable,
  apiGetLANPrinterStatus,
  apiGetPrinters,
  apiGetTroubleshootInfo,
  apiGetWebViewConfig,
  apiRemoveLANPrinter,
  apiSetWebViewEnabled,
  apiSetWebViewURL,
  apiTestPrint,
  ApiAppVariable,
  ApiPrinter,
  ApiPrintersResponse,
  ApiTroubleshootInfo,
  ApiWebViewConfig,
} from "../api/client";
import { executePrint } from "../functions/executePrint";
import { main } from "../../wailsjs/go/models";

export interface IBackendService {
  readonly isWails: boolean;

  // App & System Info
  getAppVariable(): Promise<main.AppVariable | ApiAppVariable>;
  getTroubleshootInfo(): Promise<main.TroubleshootInfo | ApiTroubleshootInfo>;
  getNetworkPrintingEnabled(): Promise<boolean>;
  setNetworkPrintingEnabled(v: boolean): Promise<void>;
  getAutostart(): Promise<boolean>;
  setAutostart(v: boolean): Promise<void>;

  // Printers
  getPrinters(): Promise<main.Printers | ApiPrintersResponse>;
  checkLANPrinterStatus(ip: string): Promise<{ online: boolean }>;
  addLANPrinter(ip: string): Promise<void>;
  removeLANPrinter(ip: string): Promise<boolean>;
  testPrint(printer: main.Printer | ApiPrinter): Promise<void>;
  openCashDrawer(printer: main.Printer | ApiPrinter): Promise<void>;

  // Kiosk & Webview
  getWebViewConfig(): Promise<main.WebViewConfig | ApiWebViewConfig>;
  setWebViewURL(url: string): Promise<void>;
  setWebViewEnabled(enabled: boolean): Promise<void>;
  setWebViewPIN(pin: string): Promise<void>;
  validatePIN(pin: string): Promise<boolean>;
  setWindowFullscreen(fullscreen: boolean): Promise<void>;
}

class WailsBackendService implements IBackendService {
  readonly isWails = true;

  getAppVariable(): Promise<main.AppVariable> {
    return AppVariable();
  }

  getTroubleshootInfo(): Promise<main.TroubleshootInfo> {
    return GetTroubleshootInfo();
  }

  getNetworkPrintingEnabled(): Promise<boolean> {
    return IsNetworkPrintingEnabled();
  }

  setNetworkPrintingEnabled(v: boolean): Promise<void> {
    return SetNetworkPrintingEnabled(v);
  }

  getAutostart(): Promise<boolean> {
    return IsAutostartEnabled();
  }

  async setAutostart(v: boolean): Promise<void> {
    if (v) {
      await EnableAutostart();
    } else {
      await DisableAutostart();
    }
  }

  getPrinters(): Promise<main.Printers> {
    return Printers();
  }

  async checkLANPrinterStatus(ip: string): Promise<{ online: boolean }> {
    const online = await CheckLANPrinterStatus(ip);
    return { online: Boolean(online) };
  }

  addLANPrinter(ip: string): Promise<void> {
    return AddLANPrinter(ip);
  }

  async removeLANPrinter(ip: string): Promise<boolean> {
    const confirmed = await ConfirmRemoveLANPrinter(ip);
    return Boolean(confirmed);
  }

  async testPrint(printer: main.Printer | ApiPrinter): Promise<void> {
    await executePrint(printer as main.Printer);
  }

  async openCashDrawer(printer: main.Printer | ApiPrinter): Promise<void> {
    await executePrint(printer as main.Printer, true);
  }

  getWebViewConfig(): Promise<main.WebViewConfig> {
    return GetWebViewConfig();
  }

  setWebViewURL(url: string): Promise<void> {
    return SetWebViewURL(url);
  }

  setWebViewEnabled(enabled: boolean): Promise<void> {
    return SetWebViewEnabled(enabled);
  }

  setWebViewPIN(pin: string): Promise<void> {
    return SetWebViewPIN(pin);
  }

  validatePIN(pin: string): Promise<boolean> {
    return ValidateWebViewPIN(pin);
  }

  setWindowFullscreen(fullscreen: boolean): Promise<void> {
    return SetWindowFullscreen(fullscreen);
  }
}

class RemoteBackendService implements IBackendService {
  readonly isWails = false;

  getAppVariable(): Promise<ApiAppVariable> {
    return apiGetAppVariable();
  }

  getTroubleshootInfo(): Promise<ApiTroubleshootInfo> {
    return apiGetTroubleshootInfo();
  }

  getNetworkPrintingEnabled(): Promise<boolean> {
    return Promise.resolve(false);
  }

  setNetworkPrintingEnabled(): Promise<void> {
    return Promise.resolve();
  }

  getAutostart(): Promise<boolean> {
    return Promise.resolve(false);
  }

  setAutostart(): Promise<void> {
    return Promise.resolve();
  }

  getPrinters(): Promise<ApiPrintersResponse> {
    return apiGetPrinters();
  }

  checkLANPrinterStatus(ip: string): Promise<{ online: boolean }> {
    return apiGetLANPrinterStatus(ip);
  }

  async addLANPrinter(ip: string): Promise<void> {
    await apiAddLANPrinter(ip);
  }

  async removeLANPrinter(ip: string): Promise<boolean> {
    await apiRemoveLANPrinter(ip);
    return true;
  }

  async testPrint(printer: main.Printer | ApiPrinter): Promise<void> {
    await apiTestPrint(printer.id);
  }

  async openCashDrawer(printer: main.Printer | ApiPrinter): Promise<void> {
    await apiCashDrawer(printer.id);
  }

  getWebViewConfig(): Promise<ApiWebViewConfig> {
    return apiGetWebViewConfig();
  }

  async setWebViewURL(url: string): Promise<void> {
    await apiSetWebViewURL(url);
  }

  async setWebViewEnabled(enabled: boolean): Promise<void> {
    await apiSetWebViewEnabled(enabled);
  }

  setWebViewPIN(): Promise<void> {
    return Promise.reject(new Error("PIN configuration is only permitted on the desktop application"));
  }

  validatePIN(pin: string): Promise<boolean> {
    return apiCreatePINSession(pin);
  }

  setWindowFullscreen(): Promise<void> {
    return Promise.resolve();
  }
}

export const backendService: IBackendService = detectWails()
  ? new WailsBackendService()
  : new RemoteBackendService();
