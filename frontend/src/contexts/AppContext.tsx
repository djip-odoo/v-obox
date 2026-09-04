import { createContext, useContext, useEffect, useState } from "react";
// @ts-ignore: Wails v3 generated JS bindings
import * as WailsApp from "../../bindings/epos-proxy/app.js";
import { setWailsToken } from "../api/authState";
import { backendService } from "../services/backend";
import { ApiAppVariable } from "../api/client";
import { AppVariable } from "../types/models";

export function detectWails(): boolean {
  if (typeof window === "undefined") return false;
  const w = window as unknown as Record<string, unknown>;
  const chrome = w["chrome"] as Record<string, unknown> | undefined;
  const webview = chrome?.["webview"] as Record<string, unknown> | undefined;
  const webkit = w["webkit"] as Record<string, unknown> | undefined;
  const msgHandlers = webkit?.["messageHandlers"] as Record<string, unknown> | undefined;
  const wails = w["wails"] as Record<string, unknown> | undefined;

  return Boolean(
    webview?.["postMessage"] ||
    (msgHandlers?.["external"] as Record<string, unknown> | undefined)?.["postMessage"] ||
    (msgHandlers?.["wails"] as Record<string, unknown> | undefined)?.["postMessage"] ||
    wails?.["invoke"]
  );
}

const RETRY_INTERVAL = 5000;

export type AppContextType = {
  setters: Record<string, never>;
  data: {
    isWails: boolean;
    ready: boolean;
    serverURL: string | null;
    app: AppVariable | ApiAppVariable | null;
    os: string | null;
    isWindows: boolean;
    isMac: boolean;
    isLinux: boolean;
    isAndroid: boolean;
    serverIsRunning: boolean;
  };
  actions: Record<string, never>;
};

export const AppContext = createContext({} as AppContextType);

export function useApp() {
  return useContext(AppContext);
}

interface AppContextWrapperProps {
  children: React.ReactNode;
}

export const AppContextWrapper = ({ children }: AppContextWrapperProps) => {
  const isWails = detectWails();
  const [ready, setReady] = useState(!isWails);
  const [app, setApp] = useState<AppVariable | ApiAppVariable | null>(null);
  const [serverURL] = useState<string | null>(
    isWails ? null : typeof window !== "undefined" ? window.location.origin : null,
  );

  const os = app?.os || null;
  const data = {
    isWails,
    ready,
    serverURL,
    app,
    os,
    isWindows: os === "windows",
    isMac: os === "darwin",
    isLinux: os === "linux",
    isAndroid: os === "android",
    serverIsRunning: app?.serverRunning ?? false,
  };
  const setters = {} as Record<string, never>;
  const actions = {} as Record<string, never>;

  useEffect(() => {
    if (!isWails) return;

    WailsApp.GetSessionToken()
      .then((token: string) => {
        if (token) {
          setWailsToken(token);
        }
        setReady(true);
      })
      .catch((err: unknown) => {
        console.error("Failed to get Wails session token:", err);
        setReady(true);
      });
  }, [isWails]);

  useEffect(() => {
    let cancelled = false;
    let retryId: number | null = null;

    const fetchAppContext = async () => {
      try {
        const variables = await backendService.getAppVariable();

        if (cancelled) return;
        setApp(variables);
      } catch (error) {
        console.error("Failed to fetch app context:", error);
        if (!cancelled) {
          retryId = window.setTimeout(fetchAppContext, RETRY_INTERVAL);
        }
      }
    };

    fetchAppContext();

    return () => {
      cancelled = true;
      if (retryId !== null) clearTimeout(retryId);
    };
  }, []);

  useEffect(() => {
    if (isWails && ready) {
      backendService.getDeviceInfo().catch(() => {});
    }
  }, [isWails, ready]);

  return (
    <AppContext.Provider value={{ data, setters, actions }}>
      {children}
    </AppContext.Provider>
  );
};
