import { createContext, useContext, useEffect, useState } from "react";
import { main } from "../../wailsjs/go/models";
import { GetSessionToken } from "../../wailsjs/go/main/App";
import { setWailsToken } from "../api/authState";
import { backendService } from "../services/backend";
import { ApiAppVariable } from "../api/client";

export function detectWails(): boolean {
  return (
    typeof window !== "undefined" &&
    !!(window as unknown as Record<string, unknown>)["go"]
  );
}

const RETRY_INTERVAL = 5000;

export type AppContextType = {
  setters: Record<string, never>;
  data: {
    isWails: boolean;
    ready: boolean;
    serverURL: string | null;
    app: main.AppVariable | ApiAppVariable | null;
    os: string | null;
    isWindows: boolean;
    isMac: boolean;
    isLinux: boolean;
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
  const [app, setApp] = useState<main.AppVariable | ApiAppVariable | null>(null);
  const [serverURL, setServerURL] = useState<string | null>(
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
    serverIsRunning: app?.serverRunning ?? false,
  };
  const setters = {} as Record<string, never>;
  const actions = {} as Record<string, never>;

  useEffect(() => {
    if (!isWails) return;

    GetSessionToken()
      .then((token) => {
        if (token) {
          setWailsToken(token);
        }
        setReady(true);
      })
      .catch((err) => {
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

  return (
    <AppContext.Provider value={{ data, setters, actions }}>
      {children}
    </AppContext.Provider>
  );
};
