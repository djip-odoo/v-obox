import { useContext, useState } from "react";
import { PrinterContext } from "../contexts/PrinterContext";
import { AppContext } from "../contexts/AppContext";
import { ToastContext } from "../contexts/ToastContext";
import { errorText } from "../error";

export default function OboxFields() {
  const printerContext = useContext(PrinterContext);
  const appContext = useContext(AppContext);
  const toastContext = useContext(ToastContext);

  const [copied, setCopied] = useState<string | null>(null);

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
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}