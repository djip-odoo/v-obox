import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import { Events } from "@wailsio/runtime";
import { AppContext } from "./AppContext";
import { backendService } from "../services/backend";

export type WebViewConfig = {
  url: string;
  enabled: boolean;
  hasPIN: boolean;
  zoom?: number;
  isActive?: boolean;
};

type WebViewContextType = {
  data: {
    config: WebViewConfig | null;
    isKioskActive: boolean;
    reloadNonce: number;
  };
  actions: {
    saveURL: (url: string) => Promise<void>;
    saveZoom: (zoom: number) => Promise<void>;
    savePIN: (pin: string) => Promise<void>;
    toggleEnabled: (v: boolean) => Promise<void>;
    validatePIN: (pin: string) => Promise<boolean>;
    exitKiosk: () => Promise<void>;
    enterKiosk: () => Promise<void>;
    reloadKiosk: () => void;
    openWebapp: (url?: string) => Promise<void>;
    closeWebapp: () => Promise<void>;
    refresh: () => Promise<void>;
    setDefaultLauncher: () => Promise<void>;
  };
};

export const WebViewContext = createContext({} as WebViewContextType);

interface WebViewContextWrapperProps {
  children: React.ReactNode;
}

export const WebViewContextWrapper = ({
  children,
}: WebViewContextWrapperProps) => {
  const { data: { isWails } } = useContext(AppContext);
  const [config, setConfig] = useState<WebViewConfig | null>(null);
  const [isKioskActive, setIsKioskActive] = useState(false);
  const [reloadNonce, setReloadNonce] = useState(0);
  const initialStartupChecked = useRef(false);

  const refresh = useCallback(async () => {
    try {
      const cfg = await backendService.getWebViewConfig();
      setConfig(cfg);
      setIsKioskActive(Boolean(cfg.enabled));
    } catch (err) {
      console.error("Failed to fetch WebView config:", err);
    }
  }, []);

  useEffect(() => {
    refresh();
    // Auto-refresh periodically so remote and local browser views stay in real-time sync
    const interval = setInterval(() => {
      refresh();
    }, 1500);
    return () => clearInterval(interval);
  }, [refresh]);

  // Listen for desktop events when config or kiosk state is modified
  useEffect(() => {
    if (!isWails) return;

    const unsubKiosk = Events.On("kiosk-state-changed", async (ev: { data: boolean }) => {
      const enabled = ev.data;
      setIsKioskActive(enabled);
      try {
        const cfg = await backendService.getWebViewConfig();
        setConfig(cfg);
      } catch {
        /* ignore */
      }
    });

    const unsubConfig = Events.On("webview-config-changed", async () => {
      try {
        const cfg = await backendService.getWebViewConfig();
        setConfig(cfg);
      } catch {
        /* ignore */
      }
    });

    const unsubReload = Events.On("kiosk-reload", () => {
      setReloadNonce((n) => n + 1);
    });

    return () => {
      unsubKiosk();
      unsubConfig();
      unsubReload();
    };
  }, [isWails]);

  // ── Actions ────────────────────────────────────────────────────────────────

  const saveURL = async (url: string) => {
    await backendService.setWebViewURL(url);
    const cfg = await backendService.getWebViewConfig();
    setConfig(cfg);
  };

  const saveZoom = async (zoom: number) => {
    await backendService.setWebViewZoom(zoom);
    const cfg = await backendService.getWebViewConfig();
    setConfig(cfg);
  };

  const savePIN = async (pin: string) => {
    await backendService.setWebViewPIN(pin);
    const cfg = await backendService.getWebViewConfig();
    setConfig(cfg);
  };

  const toggleEnabled = async (v: boolean) => {
    await backendService.setWebViewEnabled(v);
    if (backendService.isWails) {
      setIsKioskActive(v);
      await backendService.setWindowFullscreen(v);
    }
    const cfg = await backendService.getWebViewConfig();
    setConfig(cfg);
  };

  const enterKiosk = async () => {
    await toggleEnabled(true);
  };

  const exitKiosk = async () => {
    await toggleEnabled(false);
  };

  const reloadKiosk = async () => {
    setReloadNonce((n) => n + 1);
    try {
      await backendService.reloadKiosk();
    } catch (err) {
      console.error("Failed to trigger kiosk reload:", err);
    }
  };

  const openWebapp = async (url?: string) => {
    setConfig((prev) => (prev ? { ...prev, isActive: true } : null));
    try {
      await backendService.openWebView(url);
    } finally {
      await refresh();
    }
  };

  const closeWebapp = async () => {
    setConfig((prev) => (prev ? { ...prev, isActive: false } : null));
    try {
      await backendService.closeWebView();
    } finally {
      await refresh();
    }
  };

  const validatePIN = async (pin: string): Promise<boolean> => {
    return backendService.validatePIN(pin);
  };

  const setDefaultLauncher = async () => {
    try {
      await backendService.requestDefaultLauncher?.();
    } catch (err) {
      console.error("Failed to open default launcher settings:", err);
    }
  };

  return (
    <WebViewContext.Provider
      value={{
        data: { config, isKioskActive, reloadNonce },
        actions: {
          saveURL,
          saveZoom,
          savePIN,
          toggleEnabled,
          validatePIN,
          enterKiosk,
          exitKiosk,
          reloadKiosk,
          openWebapp,
          closeWebapp,
          refresh,
          setDefaultLauncher,
        },
      }}
    >
      {children}
    </WebViewContext.Provider>
  );
};
