import { useContext, useEffect, useMemo, useState } from "react";
import { PrinterContext } from "../contexts/PrinterContext";
import { ToastContext } from "../contexts/ToastContext";
import StepDialog from "./StepDialog";
import { GetTroubleshootInfo } from "../../wailsjs/go/main/App";
import { main } from "../../wailsjs/go/models";
import { getTroubleshootSteps } from "../assets/data/troubleshootStep";
import type { Step } from "../types";

export default function NetworkPrinting() {
  const printerContext = useContext(PrinterContext);
  const toastContext = useContext(ToastContext);
  const [isUpdating, setIsUpdating] = useState(false);
  const [info, setInfo] = useState<main.TroubleshootInfo | null>(null);

  const enabled = printerContext.data.networkPrintingEnabled;

  const fetchInfo = async () => {
    try {
      const data = await GetTroubleshootInfo();
      if (data) {
        setInfo(data);
      }
    } catch (err) {
      console.error("Failed to load troubleshoot info", err);
    }
  };

  useEffect(() => {
    if (enabled) {
      fetchInfo();
    }
  }, [enabled]);

  const steps = useMemo<Step[]>(() => {
    if (!info) {
      return [];
    }
    return getTroubleshootSteps(info);
  }, [info]);

  const handleToggle = async () => {
    if (isUpdating) return;
    setIsUpdating(true);
    try {
      const nextState = !enabled;
      const res = await printerContext.actions.toggleNetworkPrinting(nextState);
      toastContext.actions.showToast(
        res.message,
        res.status ? "success" : "danger",
      );
    } finally {
      setIsUpdating(false);
    }
  };

  return (
    <div className="mt-3 flex items-center justify-center gap-2">
      <div className="flex items-center gap-2">
        <span className="text-xs font-medium text-gray-700">
          Allow Other Devices to Print
        </span>

        <button
          onClick={handleToggle}
          type="button"
          className={`relative inline-flex h-4 w-7 items-center rounded-full transition-colors duration-200 focus:outline-none cursor-pointer shrink-0 ${
            enabled ? "bg-odoo" : "bg-gray-300"
          }`}
        >
          <span
            className={`inline-block h-3 w-3 transform rounded-full bg-white transition duration-200 ${
              enabled ? "translate-x-3.5" : "translate-x-0.5"
            }`}
          />
        </button>

        <div className="relative flex items-center group">
          {enabled ? (
            <StepDialog
              title="Troubleshoot Network Printing"
              steps={steps}
              onOpen={fetchInfo}
              openButton={
                <div
                  className="w-4 h-4 rounded-full bg-odoo hover:bg-odoo-dark text-white flex items-center justify-center cursor-pointer transition-colors shrink-0 shadow-xs"
                  title="Firewall & Network Troubleshooting Guide"
                >
                  <svg
                    className="w-2.5 h-2.5"
                    viewBox="0 0 20 20"
                    fill="currentColor"
                  >
                    <path
                      fillRule="evenodd"
                      d="M4 4a2 2 0 012-2h4.586A2 2 0 0112 2.586L15.414 6A2 2 0 0116 7.414V16a2 2 0 01-2 2H6a2 2 0 01-2-2V4zm2 6a1 1 0 011-1h6a1 1 0 110 2H7a1 1 0 01-1-1zm1 3a1 1 0 100 2h6a1 1 0 100-2H7z"
                      clipRule="evenodd"
                    />
                  </svg>
                </div>
              }
            />
          ) : (
            <svg
              className="w-3.5 h-3.5 text-gray-400 hover:text-gray-600 transition-colors cursor-help block"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth="2"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
              />
            </svg>
          )}
          <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-1.5 hidden group-hover:block w-48 bg-gray-900/95 text-white text-[10px] rounded py-1 px-2 shadow-md text-center leading-normal z-50 pointer-events-none">
            {enabled
              ? "Showing local network IP. Click on icon for firewall guide."
              : "Showing localhost (127.0.0.1). Accessible only from this computer."}
            <div className="absolute top-full left-1/2 -translate-x-1/2 border-4 border-transparent border-t-gray-900/95" />
          </div>
        </div>
      </div>
    </div>
  );
}
