import { main } from "../../wailsjs/go/models";
import { EventsOn } from "../../wailsjs/runtime/runtime";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import { RuntimeContext } from "./RuntimeContext";
import { backendService } from "../services/backend";

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

  const checkLanPrinterStatus = useCallback(async (ip: string) => {
    if (pendingLanChecks.current.has(ip)) return;
    pendingLanChecks.current.add(ip);
    setLanStatus((prev) =>
      prev[ip] === undefined ? { ...prev, [ip]: "loading" } : prev,
    );
    try {
      const result = await backendService.checkLANPrinterStatus(ip);
      setLanStatus((prev) => ({ ...prev, [ip]: result.online ? "online" : "offline" }));
    } catch (error) {
      console.error(`Failed to check LAN printer status for ${ip}:`, error);
    } finally {
      pendingLanChecks.current.delete(ip);
    }
  }, []);

  const checkAppStatus = useCallback(
    async (force = false) => {
      if (statusChecksInFlight.current > 0 && !force) return;
      statusChecksInFlight.current++;
      try {
        const data = (await backendService.getPrinters()) as unknown as main.Printers;
        setPrinters(data);
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
    [checkLanPrinterStatus],
  );

  const removeLanPrinter = async (
    printer: main.Printer,
  ): Promise<ActionStatus> => {
    if (!printer.isLAN || !printer.lanIp) {
      return { status: false, message: "Cannot remove a non-LAN printer" };
    }
    try {
      const confirmed = await backendService.removeLANPrinter(printer.lanIp);
      if (!confirmed) throw new Error("User cancelled the removal");
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

  const addLanPrinter = async (ip: string): Promise<ActionStatus> => {
    try {
      await backendService.addLANPrinter(ip);
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

  // Network printing state
  const loadNetworkPrintingStatus = useCallback(async () => {
    try {
      const enabled = await backendService.getNetworkPrintingEnabled();
      setNetworkPrintingEnabledState(enabled);
    } catch (err) {
      console.error("Failed to load network printing status", err);
    }
  }, []);

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
