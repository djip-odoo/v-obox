import { createContext, useCallback, useEffect, useState } from "react";
import { main } from "../../wailsjs/go/models";
import { OdooStatus } from "../../wailsjs/go/main/App";

const POLL_INTERVAL = 1500;

export type OdooContextType = {
  data: {
    status: main.OdooStatus | null;
    isConnected: boolean;
  };
  actions: {
    refreshStatus: () => Promise<void>;
  };
};

export const OdooContext = createContext({} as OdooContextType);

interface OdooContextWrapperProps {
  children: React.ReactNode;
}

export const OdooContextWrapper = ({ children }: OdooContextWrapperProps) => {
  const [status, setStatus] = useState<main.OdooStatus | null>(null);

  const refreshStatus = useCallback(async () => {
    try {
      const odooStatus = await OdooStatus();
      setStatus(odooStatus);
    } catch (error) {
      console.error("Failed to fetch Odoo status:", error);
    }
  }, []);

  useEffect(() => {
    let intervalId: number | null = null;

    const startPolling = () => {
      if (intervalId !== null) {
        return;
      }
      refreshStatus();
      intervalId = window.setInterval(refreshStatus, POLL_INTERVAL);
    };

    const stopPolling = () => {
      if (intervalId === null) {
        return;
      }
      clearInterval(intervalId);
      intervalId = null;
    };

    const handleVisibilityChange = () =>
      document.hidden ? stopPolling() : startPolling();

    document.addEventListener("visibilitychange", handleVisibilityChange);
    window.addEventListener("focus", startPolling);
    window.addEventListener("blur", stopPolling);

    if (!document.hidden) {
      startPolling();
    }

    return () => {
      stopPolling();
      document.removeEventListener("visibilitychange", handleVisibilityChange);
      window.removeEventListener("focus", startPolling);
      window.removeEventListener("blur", stopPolling);
    };
  }, [refreshStatus]);

  const isConnected = Boolean(status?.connected && status?.dbUrl);

  const data = {
    status,
    isConnected,
  };

  const actions = {
    refreshStatus,
  };

  return (
    <OdooContext.Provider value={{ data, actions }}>
      {children}
    </OdooContext.Provider>
  );
};
