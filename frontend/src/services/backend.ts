import { detectWails } from "../contexts/AppContext";
// @ts-ignore: Wails v3 generated JS bindings
import * as WailsApp from "../../bindings/epos-proxy/app.js";
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
  apiReloadKiosk,
} from "../api/client";
import { executePrint } from "../functions/executePrint";
import {
  AppVariable,
  Printer,
  Printers,
  TroubleshootInfo,
  WebViewConfig,
} from "../types/models";

export interface IBackendService {
  readonly isWails: boolean;

  // App & System Info
  getAppVariable(): Promise<AppVariable | ApiAppVariable>;
  getTroubleshootInfo(): Promise<TroubleshootInfo | ApiTroubleshootInfo>;
  getNetworkPrintingEnabled(): Promise<boolean>;
  setNetworkPrintingEnabled(v: boolean): Promise<void>;
  getAutostart(): Promise<boolean>;
  setAutostart(v: boolean): Promise<void>;

  // Printers
  getPrinters(): Promise<Printers | ApiPrintersResponse>;
  checkLANPrinterStatus(ip: string): Promise<{ online: boolean }>;
  addLANPrinter(ip: string): Promise<void>;
  removeLANPrinter(ip: string): Promise<boolean>;
  testPrint(printer: Printer | ApiPrinter): Promise<void>;
  openCashDrawer(printer: Printer | ApiPrinter): Promise<void>;

  // Kiosk & Webview
  getWebViewConfig(): Promise<WebViewConfig | ApiWebViewConfig>;
  setWebViewURL(url: string): Promise<void>;
  setWebViewEnabled(enabled: boolean): Promise<void>;
  setWebViewPIN(pin: string): Promise<void>;
  validatePIN(pin: string): Promise<boolean>;
  setWindowFullscreen(fullscreen: boolean): Promise<void>;
  reloadKiosk(): Promise<void>;
  quitApp(): Promise<void>;
}

class WailsBackendService implements IBackendService {
  readonly isWails = true;

  async getAppVariable(): Promise<AppVariable> {
    const res = await WailsApp.AppVariable();
    return res as unknown as AppVariable;
  }

  async getTroubleshootInfo(): Promise<TroubleshootInfo> {
    const res = await WailsApp.GetTroubleshootInfo();
    return res as unknown as TroubleshootInfo;
  }

  async getNetworkPrintingEnabled(): Promise<boolean> {
    return await WailsApp.IsNetworkPrintingEnabled();
  }

  async setNetworkPrintingEnabled(v: boolean): Promise<void> {
    await WailsApp.SetNetworkPrintingEnabled(v);
  }

  async getAutostart(): Promise<boolean> {
    return await WailsApp.IsAutostartEnabled();
  }

  async setAutostart(v: boolean): Promise<void> {
    if (v) {
      await WailsApp.EnableAutostart();
    } else {
      await WailsApp.DisableAutostart();
    }
  }

  async getPrinters(): Promise<Printers> {
    const p = await WailsApp.Printers();
    return (p ?? { printers: [], unavailablePrinters: [], errorMsg: "" }) as unknown as Printers;
  }

  async checkLANPrinterStatus(ip: string): Promise<{ online: boolean }> {
    const online = await WailsApp.CheckLANPrinterStatus(ip);
    return { online: Boolean(online) };
  }

  async addLANPrinter(ip: string): Promise<void> {
    await WailsApp.AddLANPrinter(ip);
  }

  async removeLANPrinter(ip: string): Promise<boolean> {
    const confirmed = await WailsApp.ConfirmRemoveLANPrinter(ip);
    return Boolean(confirmed);
  }

  async testPrint(printer: Printer | ApiPrinter): Promise<void> {
    await executePrint(printer as Printer);
  }

  async openCashDrawer(printer: Printer | ApiPrinter): Promise<void> {
    await executePrint(printer as Printer, true);
  }

  async getWebViewConfig(): Promise<WebViewConfig> {
    const res = await WailsApp.GetWebViewConfig();
    return res as unknown as WebViewConfig;
  }

  async setWebViewURL(url: string): Promise<void> {
    await WailsApp.SetWebViewURL(url);
  }

  async setWebViewEnabled(enabled: boolean): Promise<void> {
    await WailsApp.SetWebViewEnabled(enabled);
  }

  async setWebViewPIN(pin: string): Promise<void> {
    await WailsApp.SetWebViewPIN(pin);
  }

  async validatePIN(pin: string): Promise<boolean> {
    return await WailsApp.ValidateWebViewPIN(pin);
  }

  async setWindowFullscreen(fullscreen: boolean): Promise<void> {
    const w = window as unknown as Record<string, unknown>;
    const wails = w["wails"] as Record<string, unknown> | undefined;
    if (typeof wails?.["setFullscreen"] === "function") {
      (wails["setFullscreen"] as (v: boolean) => void)(fullscreen);
    }
    await WailsApp.SetWindowFullscreen(fullscreen);
  }

  async reloadKiosk(): Promise<void> {
    await WailsApp.ReloadKiosk();
  }

  async quitApp(): Promise<void> {
    const w = window as unknown as Record<string, unknown>;
    const wails = w["wails"] as Record<string, unknown> | undefined;
    if (typeof wails?.["quitApp"] === "function") {
      (wails["quitApp"] as () => void)();
      return;
    }
    await WailsApp.QuitApp();
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

  async testPrint(printer: Printer | ApiPrinter): Promise<void> {
    await apiTestPrint(printer.id);
  }

  async openCashDrawer(printer: Printer | ApiPrinter): Promise<void> {
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

  setWindowFullscreen(fullscreen: boolean): Promise<void> {
    const w = window as unknown as Record<string, unknown>;
    const wails = w["wails"] as Record<string, unknown> | undefined;
    if (typeof wails?.["setFullscreen"] === "function") {
      (wails["setFullscreen"] as (v: boolean) => void)(fullscreen);
    }
    return Promise.resolve();
  }

  async reloadKiosk(): Promise<void> {
    await apiReloadKiosk();
  }

  quitApp(): Promise<void> {
    const w = window as unknown as Record<string, unknown>;
    const wails = w["wails"] as Record<string, unknown> | undefined;
    if (typeof wails?.["quitApp"] === "function") {
      (wails["quitApp"] as () => void)();
    }
    return Promise.resolve();
  }
}

class DynamicBackendService implements IBackendService {
  private readonly wails = new WailsBackendService();
  private readonly remote = new RemoteBackendService();

  private get active(): IBackendService {
    return detectWails() ? this.wails : this.remote;
  }

  get isWails(): boolean {
    return detectWails();
  }

  getAppVariable() { return this.active.getAppVariable(); }
  getTroubleshootInfo() { return this.active.getTroubleshootInfo(); }
  getNetworkPrintingEnabled() { return this.active.getNetworkPrintingEnabled(); }
  setNetworkPrintingEnabled(v: boolean) { return this.active.setNetworkPrintingEnabled(v); }
  getAutostart() { return this.active.getAutostart(); }
  setAutostart(v: boolean) { return this.active.setAutostart(v); }
  getPrinters() { return this.active.getPrinters(); }
  checkLANPrinterStatus(ip: string) { return this.active.checkLANPrinterStatus(ip); }
  addLANPrinter(ip: string) { return this.active.addLANPrinter(ip); }
  removeLANPrinter(ip: string) { return this.active.removeLANPrinter(ip); }
  testPrint(printer: Printer | ApiPrinter) { return this.active.testPrint(printer); }
  openCashDrawer(printer: Printer | ApiPrinter) { return this.active.openCashDrawer(printer); }
  getWebViewConfig() { return this.active.getWebViewConfig(); }
  setWebViewURL(url: string) { return this.active.setWebViewURL(url); }
  setWebViewEnabled(enabled: boolean) { return this.active.setWebViewEnabled(enabled); }
  setWebViewPIN(pin: string) { return this.active.setWebViewPIN(pin); }
  validatePIN(pin: string) { return this.active.validatePIN(pin); }
  setWindowFullscreen(fullscreen: boolean) {
    const w = window as unknown as Record<string, unknown>;
    const wails = w["wails"] as Record<string, unknown> | undefined;
    if (typeof wails?.["setFullscreen"] === "function") {
      (wails["setFullscreen"] as (v: boolean) => void)(fullscreen);
    }
    return this.active.setWindowFullscreen(fullscreen);
  }
  reloadKiosk() { return this.active.reloadKiosk(); }
  quitApp() { return this.active.quitApp(); }
}

export const backendService: IBackendService = new DynamicBackendService();
