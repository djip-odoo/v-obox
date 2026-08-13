import { useContext, useState } from "react";
import { PrinterContext } from "../contexts/PrinterContext";
import { AppContext } from "../contexts/AppContext";
import { OdooContext } from "../contexts/OdooContext";
import { ToastContext } from "../contexts/ToastContext";
import { errorText } from "../error";

export default function OboxFields() {
  const odooContext = useContext(OdooContext);
  const printerContext = useContext(PrinterContext);
  const appContext = useContext(AppContext);
  const toastContext = useContext(ToastContext);

  const [copied, setCopied] = useState<string | null>(null);

  const odooStatus = odooContext.data.status;
  const isOdooConnected = odooContext.data.isConnected;

  const appId =
    printerContext.data.printers?.appId || appContext.data.app?.appId || "";
  const ipAddress =
    printerContext.data.printers?.ipAddress || appContext.data.defaultIp || "";

  const copyToClipboard = async (text: string, label: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(label);
      toastContext.actions.showToast(`${label} copied to clipboard!`, "success");

      setTimeout(() => setCopied(null), 2000);
    } catch (err) {
      toastContext.actions.showToast(
        `Copy failed: ${errorText(err, "unknown error")}`,
        "danger",
      );
    }
  };

  const renderCopyButton = (value: string, label: string) => (
    <button
      type="button"
      onClick={() => copyToClipboard(value, label)}
      title={copied === label ? "Copied!" : `Copy ${label}`}
      aria-label={`Copy ${label}`}
      className={`shrink-0 p-1.5 rounded-lg transition-colors cursor-pointer flex items-center justify-center ${
        copied === label
          ? "text-success bg-green-50"
          : "text-gray-400 hover:text-odoo hover:bg-gray-100"
      }`}
    >
      {copied === label ? (
        <svg
          className="w-4 h-4"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth="2.5"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M5 13l4 4L19 7"
          />
        </svg>
      ) : (
        <svg
          className="w-4 h-4"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth="2"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
          />
        </svg>
      )}
    </button>
  );

  // If connected with Odoo, display Odoo db_url and websocket status
  if (isOdooConnected && odooStatus?.dbUrl) {
    const wsStatus = odooStatus.websocketStatus || "connected";
    return (
      <div className="px-4 sm:px-6 py-3 border-b border-gray-200 bg-gray-50/50">
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {/* Odoo Database URL */}
          <div>
            <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-1">
              Odoo Database URL
            </div>
            <div className="flex items-center gap-2">
              <span
                className="flex-1 min-w-0 text-sm font-mono text-gray-800 truncate"
                title={odooStatus.dbUrl}
              >
                {odooStatus.dbUrl}
              </span>
              {renderCopyButton(odooStatus.dbUrl, "Odoo Database URL")}
            </div>
          </div>

          {/* Websocket Status & Disconnect Button */}
          <div>
            <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-1">
              Websocket Status
            </div>
            <div className="flex items-center justify-between gap-2 h-8">
              {wsStatus === "connected" ? (
                <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-emerald-100 text-emerald-800">
                  <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
                  Connected
                </span>
              ) : wsStatus === "connecting" || wsStatus === "polling" ? (
                <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-amber-100 text-amber-800">
                  <span className="w-2 h-2 rounded-full bg-amber-500 animate-pulse" />
                  Connecting
                </span>
              ) : (
                <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-rose-100 text-rose-800">
                  <span className="w-2 h-2 rounded-full bg-rose-500" />
                  Disconnected
                </span>
              )}

              <button
                type="button"
                onClick={async () => {
                  const removed = await odooContext.actions.disconnectOdoo();
                  if (removed) {
                    toastContext.actions.showToast(
                      "Odoo connection removed",
                      "success",
                    );
                  }
                }}
                className="text-xs text-red-600 hover:text-red-700 hover:bg-red-50 border border-red-200 px-2.5 py-1 rounded-md transition font-medium cursor-pointer flex items-center gap-1 shrink-0"
                title="Remove Odoo Connection"
              >
                <svg
                  className="w-3.5 h-3.5"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  strokeWidth="2"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                  />
                </svg>
                <span>Disconnect</span>
              </button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // If not connected with Odoo, display App ID and IP Address
  if (!appId && !ipAddress) {
    return null;
  }

  const fields = [
    { label: "App ID", value: appId },
    { label: "IP Address", value: ipAddress },
  ].filter((field) => field.value);

  return (
    <div className="px-4 sm:px-6 py-3 border-b border-gray-200">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        {fields.map(({ label, value }) => (
          <div key={label}>
            <div className="text-xs text-gray-500 mb-1">{label}</div>
            <div className="flex items-center gap-2">
              <span
                className="flex-1 min-w-0 text-sm font-mono text-gray-800 truncate"
                title={value}
              >
                {value}
              </span>
              {renderCopyButton(value, label)}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}