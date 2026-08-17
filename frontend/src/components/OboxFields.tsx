import { useContext } from "react";
import { AppContext } from "../contexts/AppContext";
import { OdooContext } from "../contexts/OdooContext";
import { ToastContext } from "../contexts/ToastContext";
import { useClipboard } from "../hooks/useClipboard";

interface CopyButtonProps {
  label: string;
  isCopied: boolean;
  onCopy: () => void;
}

function CopyButton({ label, isCopied, onCopy }: CopyButtonProps) {
  return (
    <button
      type="button"
      onClick={onCopy}
      title={isCopied ? "Copied!" : `Copy ${label}`}
      aria-label={`Copy ${label}`}
      className={`shrink-0 p-1.5 rounded-lg transition-all duration-150 cursor-pointer flex items-center justify-center ${
        isCopied
          ? "text-emerald-600 bg-emerald-100/80 ring-1 ring-emerald-300"
          : "text-gray-400 hover:text-odoo hover:bg-purple-50 active:scale-95"
      }`}
    >
      {isCopied ? (
        <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2.5">
          <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
        </svg>
      ) : (
        <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2">
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
          />
        </svg>
      )}
    </button>
  );
}

interface FieldCardProps {
  label: string;
  value: string;
  title: string;
  iconBg: string;
  icon: React.ReactNode;
  isCopied: boolean;
  onCopy: () => void;
}

function FieldCard({ label, value, title, iconBg, icon, isCopied, onCopy }: FieldCardProps) {
  return (
    <div
      className="flex items-center justify-between gap-2 bg-gray-50/80 hover:bg-gray-50 border border-gray-200/70 rounded-xl px-3 py-2 transition-all shadow-2xs"
      title={title}
    >
      <div className="flex items-center gap-2.5 min-w-0">
        <div className={`w-7 h-7 rounded-lg ${iconBg} flex items-center justify-center shrink-0 border`}>
          {icon}
        </div>
        <div className="min-w-0">
          <div className="text-[10px] font-semibold text-gray-400 uppercase tracking-wider">
            {label}
          </div>
          <div className="font-mono text-xs text-gray-800 font-medium truncate">
            {value}
          </div>
        </div>
      </div>
      <CopyButton label={label} isCopied={isCopied} onCopy={onCopy} />
    </div>
  );
}

export default function OboxFields() {
  const appContext = useContext(AppContext);
  const odooContext = useContext(OdooContext);
  const toastContext = useContext(ToastContext);
  const { copy, isCopied } = useClipboard();

  const odooStatus = odooContext.data.status;
  const isOdooConnected = odooContext.data.isConnected;

  const appId = odooStatus?.appId || appContext.data.app?.appId || "";
  const ipAddress = odooStatus?.ipAddress || "";

  // Odoo Connected State
  if (isOdooConnected && odooStatus?.dbUrl) {
    const wsStatus = odooStatus.websocketStatus || "connected";

    return (
      <div className="pb-3 border-b border-gray-100">
        <div className="rounded-xl border border-purple-100 bg-gradient-to-br from-purple-50/70 via-white to-purple-50/40 p-3 shadow-xs">
          {/* Header row: Database status & live WS */}
          <div className="flex items-center justify-between gap-3 mb-2">
            <div className="flex items-center gap-1.5">
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-[11px] font-semibold uppercase tracking-wider bg-odoo text-white shadow-xs">
                <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2.5">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                </svg>
                Odoo Connected
              </span>
            </div>

            <div className="flex items-center gap-2">
              {/* WebSocket Status Pill */}
              <div
                className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium border ${
                  wsStatus === "connected"
                    ? "bg-emerald-50 text-emerald-700 border-emerald-200/70"
                    : wsStatus === "connecting" || wsStatus === "polling"
                    ? "bg-amber-50 text-amber-700 border-amber-200/70"
                    : "bg-rose-50 text-rose-700 border-rose-200/70"
                }`}
                title={`WebSocket Status: ${wsStatus}`}
              >
                <span className="relative flex h-2 w-2">
                  {wsStatus === "connected" ? (
                    <>
                      <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-60" />
                      <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500" />
                    </>
                  ) : wsStatus === "connecting" || wsStatus === "polling" ? (
                    <span className="relative inline-flex rounded-full h-2 w-2 bg-amber-500 animate-pulse" />
                  ) : (
                    <span className="relative inline-flex rounded-full h-2 w-2 bg-rose-500" />
                  )}
                </span>
                <span className="capitalize">{wsStatus === "connected" ? "Live" : wsStatus}</span>
              </div>

              {/* Disconnect Button */}
              <button
                type="button"
                onClick={async () => {
                  const removed = await odooContext.actions.disconnectOdoo();
                  if (removed) {
                    toastContext.actions.showToast("Odoo connection removed", "success");
                  }
                }}
                className="p-1 rounded-lg text-gray-400 hover:text-rose-600 hover:bg-rose-50 transition-colors cursor-pointer"
                title="Disconnect from Odoo"
                aria-label="Disconnect from Odoo"
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"
                  />
                </svg>
              </button>
            </div>
          </div>

          {/* Database URL pill */}
          <div
            className="flex items-center justify-between gap-2 bg-white/90 border border-purple-100/80 rounded-lg px-2.5 py-1.5"
            title="Odoo Database URL"
          >
            <div className="flex items-center gap-2 min-w-0">
              <svg className="w-3.5 h-3.5 text-odoo shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1"
                />
              </svg>
              <span className="font-mono text-xs text-gray-800 font-medium truncate">
                {odooStatus.dbUrl}
              </span>
            </div>
            <CopyButton
              label="Database URL"
              isCopied={isCopied("Database URL")}
              onCopy={() => copy(odooStatus.dbUrl, "Database URL")}
            />
          </div>
        </div>
      </div>
    );
  }

  // Disconnected / Standalone State
  if (!appId && !ipAddress) {
    return null;
  }

  return (
    <div className="pb-3 border-b border-gray-100">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
        {/* Device / App ID Card */}
        {appId && (
          <FieldCard
            label="App ID"
            value={appId}
            title="App ID (Hardware Serial)"
            iconBg="bg-purple-50 text-odoo border-purple-100/60"
            icon={
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z"
                />
              </svg>
            }
            isCopied={isCopied("App ID")}
            onCopy={() => copy(appId, "App ID")}
          />
        )}

        {/* Local IP Card */}
        {ipAddress && (
          <FieldCard
            label="Local IP"
            value={ipAddress}
            title="Local Network IP"
            iconBg="bg-blue-50 text-blue-600 border-blue-100/60"
            icon={
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9"
                />
              </svg>
            }
            isCopied={isCopied("IP Address")}
            onCopy={() => copy(ipAddress, "IP Address")}
          />
        )}
      </div>
    </div>
  );
}


