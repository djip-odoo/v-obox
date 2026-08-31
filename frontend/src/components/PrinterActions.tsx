import { useContext, useState } from "react";
import { ToastContext } from "../contexts/ToastContext";
import { Printer } from "../types/models";
import { errorText } from "../error";
import { usePINGate } from "../hooks/usePINGate";
import { useClipboard } from "../hooks/useClipboard";
import { backendService } from "../services/backend";

interface PrinterActionsProps {
  printer: Printer;
}

export default function PrinterActions({ printer }: PrinterActionsProps) {
  const toastContext = useContext(ToastContext);
  const gate = usePINGate();
  const [isTestPrinting, setIsTestPrinting] = useState(false);
  const [isCashDrawerOpening, setIsCashDrawerOpening] = useState(false);

  const { copied: copiedIp, copy: onCopy } = useClipboard({
    successMessage: "Printer IP copied to clipboard",
    errorMessage: "Copy failed",
  });

  async function onTest() {
    setIsTestPrinting(true);
    try {
      const res = await gate(async () => {
        await backendService.testPrint(printer);
        return true;
      });
      if (res) {
        toastContext.actions.showToast(
          `Test print sent to ${printer.name}`,
          "success",
        );
      }
    } catch (err) {
      toastContext.actions.showToast(
        errorText(err, "Test print failed"),
        "danger",
      );
    } finally {
      setIsTestPrinting(false);
    }
  }

  async function onCashDrawerOpen() {
    setIsCashDrawerOpening(true);
    try {
      const res = await gate(async () => {
        await backendService.openCashDrawer(printer);
        return true;
      });
      if (res) {
        toastContext.actions.showToast(
          `Cash drawer opened for ${printer.name}`,
          "success",
        );
      }
    } catch (err) {
      toastContext.actions.showToast(
        errorText(err, "Failed to open the cash drawer"),
        "danger",
      );
    } finally {
      setIsCashDrawerOpening(false);
    }
  }

  return (
    <div className="flex gap-2 mt-3 flex-wrap">
      {/* Copy IP */}
      <button
        onClick={() => onCopy(printer.ip)}
        className={`flex-1 min-w-[5.5rem] border text-xs sm:text-sm rounded-lg px-2.5 py-1.5 sm:py-2 cursor-pointer whitespace-nowrap transition-colors ${
          copiedIp
            ? "bg-success text-white border-transparent"
            : "bg-odoo text-white border-transparent hover:bg-odoo-dark"
        }`}
      >
        {copiedIp ? "✓ Copied!" : "Copy IP"}
      </button>

      {/* Test Print */}
      <button
        onClick={onTest}
        disabled={isTestPrinting}
        className="flex-1 min-w-[5rem] border rounded-lg text-xs sm:text-sm px-2.5 py-1.5 sm:py-2 cursor-pointer border-gray-300 text-gray-700 hover:bg-gray-50 hover:border-gray-400 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        {isTestPrinting ? "Printing..." : "Test"}
      </button>

      {/* Cash Drawer */}
      {printer.type === "receipt" && (
        <button
          onClick={onCashDrawerOpen}
          disabled={isCashDrawerOpening}
          className="flex-1 min-w-[6.5rem] border rounded-lg text-xs sm:text-sm px-2.5 py-1.5 sm:py-2 cursor-pointer border-gray-300 text-gray-700 hover:bg-gray-50 hover:border-gray-400 disabled:opacity-50 disabled:cursor-not-allowed transition-colors whitespace-nowrap"
        >
          {isCashDrawerOpening ? "Opening..." : "Cash Drawer"}
        </button>
      )}
    </div>
  );
}
