import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import {
  GetWebViewConfig,
  SetWebViewEnabled,
  SetWebViewPIN,
  SetWebViewURL,
  SetWindowFullscreen,
  ValidateWebViewPIN,
} from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import {
  apiCreatePINSession,
  apiGetWebViewConfig,
  apiSetWebViewEnabled,
  apiSetWebViewURL,
} from "../api/client";
import { RuntimeContext } from "./RuntimeContext";

export type WebViewConfig = {
  url: string;
  enabled: boolean;
  hasPIN: boolean;
};

type WebViewContextType = {
  data: {
    config: WebViewConfig | null;
    isKioskActive: boolean;
    reloadNonce: number;
  };
  actions: {
    saveURL: (url: string) => Promise<void>;
    savePIN: (pin: string) => Promise<void>;
    toggleEnabled: (v: boolean) => Promise<void>;
    validatePIN: (pin: string) => Promise<boolean>;
    exitKiosk: () => Promise<void>;
    enterKiosk: () => Promise<void>;
    reloadKiosk: () => void;
    refresh: () => Promise<void>;
  };
};

export const WebViewContext = createContext({} as WebViewContextType);

interface WebViewContextWrapperProps {
  children: React.ReactNode;
}

export const WebViewContextWrapper = ({
  children,
}: WebViewContextWrapperProps) => {
  const { isWails } = useContext(RuntimeContext);
  const [config, setConfig] = useState<WebViewConfig | null>(null);
  const [isKioskActive, setIsKioskActive] = useState(false);
  const [reloadNonce, setReloadNonce] = useState(0);
  const initialStartupChecked = useRef(false);

  const refresh = useCallback(async () => {
    try {
      const cfg = isWails
        ? await GetWebViewConfig()
        : await apiGetWebViewConfig();
      setConfig(cfg);

      // Auto-activate kiosk ONCE on desktop initial startup if enabled in config
      if (
        !initialStartupChecked.current &&
        isWails &&
        cfg.enabled &&
        cfg.url
      ) {
        initialStartupChecked.current = true;
        setIsKioskActive(true);
        await SetWindowFullscreen(true);
      } else {
        initialStartupChecked.current = true;
      }
    } catch (err) {
      console.error("Failed to fetch WebView config:", err);
    }
  }, [isWails]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  // Listen for desktop events when config or kiosk state is modified
  useEffect(() => {
    if (!isWails) return;

    const unsubKiosk = EventsOn("kiosk-state-changed", async (enabled: boolean) => {
      if (enabled) {
        setIsKioskActive(true);
        await SetWindowFullscreen(true);
      } else {
        setIsKioskActive(false);
        await SetWindowFullscreen(false);
      }
      try {
        const cfg = await GetWebViewConfig();
        setConfig(cfg);
      } catch {
        /* ignore */
      }
    });

    const unsubConfig = EventsOn("webview-config-changed", async () => {
      try {
        const cfg = await GetWebViewConfig();
        setConfig(cfg);
      } catch {
        /* ignore */
      }
    });

    return () => {
      unsubKiosk();
      unsubConfig();
    };
  }, [isWails]);

  // ── Actions ────────────────────────────────────────────────────────────────

  const saveURL = async (url: string) => {
    if (isWails) {
      await SetWebViewURL(url);
    } else {
      await apiSetWebViewURL(url);
    }
    const cfg = isWails
      ? await GetWebViewConfig()
      : await apiGetWebViewConfig();
    setConfig(cfg);
  };

  const savePIN = async (pin: string) => {
    if (isWails) {
      await SetWebViewPIN(pin);
      const cfg = await GetWebViewConfig();
      setConfig(cfg);
    }
  };

  const toggleEnabled = async (v: boolean) => {
    if (isWails) {
      await SetWebViewEnabled(v);
      if (v) {
        setIsKioskActive(true);
        await SetWindowFullscreen(true);
      } else {
        setIsKioskActive(false);
        await SetWindowFullscreen(false);
      }
      const cfg = await GetWebViewConfig();
      setConfig(cfg);
    } else {
      await apiSetWebViewEnabled(v);
      const cfg = await apiGetWebViewConfig();
      setConfig(cfg);
    }
  };

  const enterKiosk = async () => {
    await toggleEnabled(true);
  };

  const exitKiosk = async () => {
    await toggleEnabled(false);
  };

  const reloadKiosk = () => {
    setReloadNonce((n) => n + 1);
  };

  const validatePIN = async (pin: string): Promise<boolean> => {
    if (isWails) {
      return ValidateWebViewPIN(pin);
    }
    return apiCreatePINSession(pin);
  };

  return (
    <WebViewContext.Provider
      value={{
        data: { config, isKioskActive, reloadNonce },
        actions: {
          saveURL,
          savePIN,
          toggleEnabled,
          validatePIN,
          enterKiosk,
          exitKiosk,
          reloadKiosk,
          refresh,
        },
      }}
    >
      {children}
    </WebViewContext.Provider>
  );
};
