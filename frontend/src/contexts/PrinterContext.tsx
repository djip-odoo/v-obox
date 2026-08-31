import { main } from "../../wailsjs/go/models";
import {
  AddLANPrinter,
  CheckLANPrinterStatus,
  IsNetworkPrintingEnabled,
  Printers,
} from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import {
  apiAddLANPrinter,
  apiGetLANPrinterStatus,
  apiGetPrinters,
  ApiPrintersResponse,
  apiRemoveLANPrinter,
} from "../api/client";
import { RuntimeContext } from "./RuntimeContext";

const POLL_INTERVAL = 5000;
const FETCH_ERROR = "Failed to retrieve printer status. Please try again.";

type PrinterLanStatusByIp = Record<string, "loading" | "online" | "offline">;

type ActionStatus = {
  status: boolean;
  message: string;
};

type PrinterContextType = {
  setters: Record<string, never>;
  data: {
    printers: main.Printers | null;
    lanStatus: PrinterLanStatusByIp;
    fetchError: string | null;
    networkPrintingEnabled: boolean;
  };
  actions: {
    removeLanPrinter: (printer: main.Printer) => Promise<ActionStatus>;
    addLanPrinter: (ip: string) => Promise<ActionStatus>;
  };
};

export const PrinterContext = createContext({} as PrinterContextType);

interface PrinterContextWrapper {
  children: React.ReactNode;
}

export const PrinterContextWrapper = ({ children }: PrinterContextWrapper) => {
  const { isWails } = useContext(RuntimeContext);
  const [printers, setPrinters] = useState<main.Printers | null>(null);
  const [lanStatus, setLanStatus] = useState<PrinterLanStatusByIp>({});
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [networkPrintingEnabled, setNetworkPrintingEnabledState] =
    useState(false);

  const statusChecksInFlight = useRef(0);
  const pendingLanChecks = useRef<Set<string>>(new Set());

  const checkLanPrinterStatus = useCallback(
    async (ip: string) => {
      if (pendingLanChecks.current.has(ip)) return;
      pendingLanChecks.current.add(ip);
      setLanStatus((prev) =>
        prev[ip] === undefined ? { ...prev, [ip]: "loading" } : prev,
      );
      try {
        let online: boolean;
        if (isWails) {
          online = await CheckLANPrinterStatus(ip);
        } else {
          const result = await apiGetLANPrinterStatus(ip);
          online = result.online;
        }
        setLanStatus((prev) => ({ ...prev, [ip]: online ? "online" : "offline" }));
      } catch (error) {
        console.error(`Failed to check LAN printer status for ${ip}:`, error);
      } finally {
        pendingLanChecks.current.delete(ip);
      }
    },
    [isWails],
  );

  const checkAppStatus = useCallback(
    async (force = false) => {
      if (statusChecksInFlight.current > 0 && !force) return;
      statusChecksInFlight.current++;
      try {
        let data: main.Printers;
        if (isWails) {
          data = await Printers();
        } else {
          data = (await apiGetPrinters()) as unknown as main.Printers;
        }
        setPrinters(data as main.Printers);
        setFetchError(null);
        for (const printer of data.printers) {
          if (printer.isLAN && printer.lanIp) {
            checkLanPrinterStatus(printer.lanIp);
          }
        }
      } catch (error) {
        console.error("Failed to check app status:", error);
        setFetchError(FETCH_ERROR);
      } finally {
        statusChecksInFlight.current--;
      }
    },
    [isWails, checkLanPrinterStatus],
  );

  // Remote context: remove LAN printer via HTTP API without a native dialog.
  // The UI shows its own confirmation (handled in PrinterListItem).
  const removeLanPrinterRemote = async (
    printer: main.Printer,
  ): Promise<ActionStatus> => {
    if (!printer.isLAN || !printer.lanIp) {
      return { status: false, message: "Cannot remove a non-LAN printer" };
    }
    try {
      await apiRemoveLANPrinter(printer.lanIp);
      await checkAppStatus(true);
      return {
        status: true,
        message: `Successfully removed LAN printer with IP ${printer.lanIp}`,
      };
    } catch (error) {
      return {
        status: false,
        message: `Failed to remove LAN printer: ${error}`,
      };
    }
  };

  // Wails context: uses native confirmation dialog (existing behaviour).
  const removeLanPrinterWails = async (
    printer: main.Printer,
  ): Promise<ActionStatus> => {
    if (!printer.isLAN || !printer.lanIp) {
      return { status: false, message: "Cannot remove a non-LAN printer" };
    }
    try {
      // Import dynamically to avoid errors in remote context where ConfirmRemoveLANPrinter
      // binding would not be available.
      const { ConfirmRemoveLANPrinter } = await import(
        "../../wailsjs/go/main/App"
      );
      const confirmed = await ConfirmRemoveLANPrinter(printer.lanIp);
      if (!confirmed) throw new Error("User cancelled the removal");
      await checkAppStatus(true);
      return {
        status: true,
        message: `Successfully removed LAN printer with IP ${printer.lanIp}`,
      };
    } catch (error) {
      return {
        status: false,
        message: `Failed to remove LAN printer with IP ${printer.lanIp}: ${error}`,
      };
    }
  };

  const removeLanPrinter = isWails
    ? removeLanPrinterWails
    : removeLanPrinterRemote;

  const addLanPrinter = async (ip: string): Promise<ActionStatus> => {
    try {
      if (isWails) {
        await AddLANPrinter(ip);
      } else {
        await apiAddLANPrinter(ip);
      }
      await checkAppStatus(true);
      return { status: true, message: `Successfully added LAN printer with IP ${ip}` };
    } catch (error) {
      return {
        status: false,
        message: `Failed to add LAN printer with IP ${ip}: ${error}`,
      };
    }
  };

  // Poll while window is visible
  useEffect(() => {
    let intervalId: number | null = null;

    const startPolling = () => {
      if (intervalId !== null) return;
      checkAppStatus();
      intervalId = window.setInterval(checkAppStatus, POLL_INTERVAL);
    };

    const stopPolling = () => {
      if (intervalId === null) return;
      clearInterval(intervalId);
      intervalId = null;
    };

    const handleVisibilityChange = () =>
      document.hidden ? stopPolling() : startPolling();

    document.addEventListener("visibilitychange", handleVisibilityChange);
    window.addEventListener("focus", startPolling);
    window.addEventListener("blur", stopPolling);

    if (!document.hidden) startPolling();

    return () => {
      stopPolling();
      document.removeEventListener("visibilitychange", handleVisibilityChange);
      window.removeEventListener("focus", startPolling);
      window.removeEventListener("blur", stopPolling);
    };
  }, [checkAppStatus]);

  // Network printing state — Wails only (excluded from remote webview)
  const loadNetworkPrintingStatus = useCallback(async () => {
    if (!isWails) return; // not exposed in remote webview
    try {
      const enabled = await IsNetworkPrintingEnabled();
      setNetworkPrintingEnabledState(enabled);
    } catch (err) {
      console.error("Failed to load network printing status", err);
    }
  }, [isWails]);

  useEffect(() => {
    loadNetworkPrintingStatus();
  }, [loadNetworkPrintingStatus]);

  // Wails event listener for network printing toggle
  useEffect(() => {
    if (!isWails) return;
    return EventsOn("network-printing-changed", () => {
      loadNetworkPrintingStatus();
      checkAppStatus(true);
    });
  }, [isWails, loadNetworkPrintingStatus, checkAppStatus]);

  const setters = {} as Record<string, never>;
  const actions = { removeLanPrinter, addLanPrinter };
  const data = { printers, lanStatus, fetchError, networkPrintingEnabled };

  return (
    <PrinterContext.Provider value={{ data, setters, actions }}>
      {children}
    </PrinterContext.Provider>
  );
};
