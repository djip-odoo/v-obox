import { useContext, useEffect, useMemo, useState } from "react";
import { PrinterContext } from "../contexts/PrinterContext";
import StepDialog from "./StepDialog";
import { backendService } from "../services/backend";
import { TroubleshootInfo } from "../types/models";
import { getLinuxSteps, getMacSteps, getWindowsSteps } from "../assets/data/troubleshootStep";
import type { Step } from "../types";
import { AppContext } from "../contexts/AppContext";

export default function NetworkPrinting() {
  const appContext = useContext(AppContext);
  const printerContext = useContext(PrinterContext);
  const [isLoadingInfo, setIsLoadingInfo] = useState(false);
  const [info, setInfo] = useState<TroubleshootInfo | null>(null);

  const enabled = printerContext.data.networkPrintingEnabled;

  const fetchInfo = async () => {
    if (!appContext.data.isWails) return;
    setIsLoadingInfo(true);
    try {
      const data = await backendService.getTroubleshootInfo();
      if (data) {
        setInfo(data);
      }
    } catch (err) {
      console.error("Failed to load troubleshoot info", err);
    } finally {
      setIsLoadingInfo(false);
    }
  };

  useEffect(() => {
    if (appContext.data.isWails && enabled) {
      fetchInfo();
    }
  }, [appContext.data.isWails, enabled]);

  const steps = useMemo<Step[]>(() => {
    if (!info) return [];
    if (appContext.data.isWindows) {
      return getWindowsSteps(info);
    }
    if (appContext.data.isLinux) {
      return getLinuxSteps(info);
    }
    if (appContext.data.isMac) {
      return getMacSteps(info);
    }
    return [];
  }, [info, appContext.data]);

  if (!appContext.data.isWails || !enabled) {
    return null;
  }

  return (
    <div className="mt-6 flex items-center justify-center">
      <StepDialog
        title="Troubleshoot Network Printing"
        steps={steps}
        isLoading={isLoadingInfo && !info}
        onOpen={fetchInfo}
        openButton={
          <span className="text-sm text-gray-600 hover:text-odoo underline-offset-2 cursor-pointer transition-colors">
            Having trouble printing?
          </span>
        }
      />
    </div>
  );
}
