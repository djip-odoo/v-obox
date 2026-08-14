import { useContext, useState } from "react";
import { AppContext } from "../contexts/AppContext";
import { OdooContext } from "../contexts/OdooContext";
import { ToastContext } from "../contexts/ToastContext";
import { errorText } from "../error";

export default function OboxFields() {
  const odooContext = useContext(OdooContext);
  const appContext = useContext(AppContext);
  const toastContext = useContext(ToastContext);

  const [copied, setCopied] = useState<string | null>(null);

  const odooStatus = odooContext.data.status;
  const isOdooConnected = odooContext.data.isConnected;

  const appId = odooStatus?.appId || appContext.data.app?.appId || "";
  const ipAddress = odooStatus?.ipAddress || appContext.data.defaultIp || "";

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

  const renderField = ( label: string, value: string,tooltip: string = "") => (
    <div
      className="flex items-center gap-2 min-w-0"
      title={tooltip}
    >
      <span className="flex-grow min-w-0 text-sm font-mono text-gray-800 truncate">
        {label} {value}
      </span>

      {renderCopyButton(value, label)}
    </div>
  );

  // Odoo connected
  if (isOdooConnected && odooStatus?.dbUrl) {
    const wsStatus = odooStatus.websocketStatus || "connected";

    return (
      <div className="px-4 sm:px-6 py-3 border-b border-gray-200">
        <div className="flex items-center gap-4">
          {/* Database URL */}
          <div className="flex-grow min-w-0">
           {renderField("Connected to ", odooStatus.dbUrl, "Odoo Database URL")}
          </div>

          {/* Websocket status */}
          <div
            className="shrink-0 flex items-center gap-2"
            title="Websocket Status"
          >
            {wsStatus === "connected" ? (
              <>
                <span className="w-2 h-2 rounded-full bg-emerald-500" />
              </>
            ) : wsStatus === "connecting" || wsStatus === "polling" ? (
              <>
                <span className="w-2 h-2 rounded-full bg-amber-500 animate-pulse" />
              </>
            ) : (
              <>
                <span className="w-2 h-2 rounded-full bg-rose-500" />
              </>
            )}
            <span className="text-xs text-gray-600">WebSocket</span>
          </div>

          {/* Disconnect */}
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
            className="shrink-0 p-1.5 rounded-lg text-gray-400 hover:text-red-600 hover:bg-red-50 transition cursor-pointer"
            title="Disconnect Odoo"
            aria-label="Disconnect Odoo"
          >
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
                d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
              />
            </svg>
          </button>
        </div>
      </div>
    );
  }

  // Not connected
  if (!appId && !ipAddress) {
    return null;
  }

  return (
    <div className="px-4 sm:px-6 py-3 border-b border-gray-200">
      <div className="flex flex-col sm:flex-row gap-3">
        {appId && (
          <div className="flex-grow min-w-0">
            {renderField("ID:", appId, "App ID")}
          </div>
        )}

        {ipAddress && (
          <div className="flex-grow min-w-0">
            {renderField("IP:", ipAddress, "IP Address")}
          </div>
        )}
      </div>
    </div>
  );
}
