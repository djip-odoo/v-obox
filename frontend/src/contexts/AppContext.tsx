import { createContext, useContext, useEffect, useState } from "react";
import { main } from "../../wailsjs/go/models";
import { AppVariable } from "../../wailsjs/go/main/App";
import { apiGetAppVariable } from "../api/client";
import { RuntimeContext } from "./RuntimeContext";

const RETRY_INTERVAL = 5000;

type AppContextType = {
  setters: Record<string, never>;
  data: {
    os: string | null;
    app: main.AppVariable | null;
    isWindows: boolean;
    isMac: boolean;
    isLinux: boolean;
    serverIsRunning: boolean;
  };
  actions: Record<string, never>;
};

export const AppContext = createContext({} as AppContextType);

interface AppContextWrapper {
  children: React.ReactNode;
}

export const AppContextWrapper = ({ children }: AppContextWrapper) => {
  const { isWails } = useContext(RuntimeContext);
  const [app, setApp] = useState<main.AppVariable | null>(null);

  const os = app?.os || null;
  const data = {
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
    let cancelled = false;
    let retryId: number | null = null;

    const fetchAppContext = async () => {
      try {
        // Wails: use the Go binding; Remote: use the HTTP API
        const variables = isWails
          ? await AppVariable()
          : await apiGetAppVariable();

        if (cancelled) return;
        setApp(variables as main.AppVariable);
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
  }, [isWails]);

  return (
    <AppContext.Provider value={{ data, setters, actions }}>
      {children}
    </AppContext.Provider>
  );
};
