import { createContext, useEffect, useState } from "react";
import { main } from "../../wailsjs/go/models";
import { backendService } from "../services/backend";

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
        const variables = await backendService.getAppVariable();

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
  }, []);

  return (
    <AppContext.Provider value={{ data, setters, actions }}>
      {children}
    </AppContext.Provider>
  );
};
