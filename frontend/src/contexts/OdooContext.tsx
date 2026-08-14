import { createContext, useCallback, useEffect, useState } from "react";
import { main } from "../../wailsjs/go/models";
import {
  ConfirmDisconnectOdoo,
  OdooStatus,
} from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";

export type OdooContextType = {
  data: {
    status: main.OdooStatus | null;
    isConnected: boolean;
  };
  actions: {
    refreshStatus: () => Promise<void>;
    disconnectOdoo: () => Promise<boolean>;
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

  const disconnectOdoo = useCallback(async () => {
    try {
      const confirmed = await ConfirmDisconnectOdoo();
      if (confirmed) {
        setStatus({
          connected: false,
          dbUrl: "",
          websocketStatus: "disconnected",
          serial: "",
          appId: "",
          ipAddress: "",
        });
        return true;
      }
      return false;
    } catch (error) {
      console.error("Failed to disconnect Odoo:", error);
      return false;
    }
  }, []);

  useEffect(() => {
    // Initial fetch on mount
    refreshStatus();

    // Listen to real-time status updates pushed from backend
    const unsubscribe = EventsOn("odoo:status_changed", (newStatus: main.OdooStatus) => {
      setStatus(newStatus);
    });

    return () => {
      if (unsubscribe) {
        unsubscribe();
      }
    };
  }, [refreshStatus]);

  const isConnected = Boolean(status?.connected && status?.dbUrl);

  const data = {
    status,
    isConnected,
  };

  const actions = {
    refreshStatus,
    disconnectOdoo,
  };

  return (
    <OdooContext.Provider value={{ data, actions }}>
      {children}
    </OdooContext.Provider>
  );
};

