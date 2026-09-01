import { useContext, useEffect, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import { WebViewContext } from "../contexts/WebViewContext";
import { AppContext } from "../contexts/AppContext";
import { ToastContext } from "../contexts/ToastContext";
import Dialog, { ActionType } from "./Dialog";
import { usePINGate } from "../hooks/usePINGate";
import { useClipboard } from "../hooks/useClipboard";
import { backendService } from "../services/backend";

/**
 * Supported URL formats for Kiosk mode.
 */
export const KIOSK_URL_CONSTRAINTS = [
  {
    type: "odoo-pos-self",
    name: "Odoo Self Order / Kiosk",
    example:
      "https://your-domain.odoo.com/pos-self/204?access_token=4fcea930b2a1479a",
    validate: (url: URL) =>
      /^\/pos-self\/[a-zA-Z0-9_-]+/i.test(url.pathname),
  },
];

export function isValidUrl(urlStr: string): boolean {
  const trimmed = urlStr.trim();

  if (!trimmed) {
    return false;
  }

  try {
    const parsed = new URL(trimmed);

    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return false;
    }

    if (!parsed.hostname) {
      return false;
    }

    return KIOSK_URL_CONSTRAINTS.some((constraint) =>
      constraint.validate(parsed)
    );
  } catch {
    return false;
  }
}

export default function WebViewDialog() {
  const { data: { isWails } } = useContext(AppContext);
  const toastContext = useContext(ToastContext);
  const { data, actions } = useContext(WebViewContext);
  const { setDefaultLauncher } = actions;
  const gate = usePINGate();
  const cfg = data.config;

  const [url, setUrl] = useState(cfg?.url ?? "");
  const [zoom, setZoom] = useState<number>(cfg?.zoom && cfg.zoom > 0 ? cfg.zoom : 1.0);
  const [localError, setLocalError] = useState<string | null>(null);
  const [serverUrl, setServerUrl] = useState(
    window.location.origin + "/"
  );
  const [reloading, setReloading] = useState(false);

  const { copied: copiedUrl, copy: copyServerUrl } = useClipboard({
    successMessage: "Server URL copied to clipboard",
    errorMessage: "Failed to copy URL",
  });

  const isUrlValid = isValidUrl(url);

  const isKioskCurrentlyActive = isWails
    ? data.isKioskActive
    : Boolean(cfg?.enabled);

  const canEnable = Boolean(isUrlValid && cfg?.hasPIN);

  /*
   * Keep URL and Zoom fields synchronized with configuration.
   */
  useEffect(() => {
    if (cfg?.url) {
      setUrl(cfg.url);
    }
    if (cfg?.zoom && cfg.zoom > 0) {
      setZoom(cfg.zoom);
    }
  }, [cfg?.url, cfg?.zoom]);

  /*
   * Load the local server address used for remote access.
   */
  useEffect(() => {
    const fetchServerInfo = async () => {
      try {
        const info = await backendService.getTroubleshootInfo();

        if (info?.localIp && info?.port) {
          setServerUrl(`http://${info.localIp}:${info.port}/`);
        } else {
          setServerUrl(window.location.origin + "/");
        }
      } catch {
        setServerUrl(window.location.origin + "/");
      }
    };

    fetchServerInfo();
  }, []);

  const handleZoomChange = async (newZoom: number) => {
    const clamped = Math.round(Math.min(2.0, Math.max(0.5, newZoom)) * 100) / 100;
    setZoom(clamped);
    try {
      await actions.saveZoom(clamped);
    } catch (err) {
      console.error("Failed to save zoom:", err);
    }
  };

  const saveSettings = async (): Promise<boolean> => {
    setLocalError(null);

    const trimmedUrl = url.trim();

    if (!trimmedUrl) {
      setLocalError("URL cannot be empty.");
      return false;
    }

    if (!isValidUrl(trimmedUrl)) {
      setLocalError(
        "Enter a valid HTTP or HTTPS URL for an Odoo POS Self Order page."
      );
      return false;
    }

    const saved = await gate(async () => {
      try {
        await actions.saveURL(trimmedUrl);
        await actions.saveZoom(zoom);
        await actions.toggleEnabled(true);

        if (isWails) {
          await actions.enterKiosk();
        }

        return true;
      } catch (err: unknown) {
        setLocalError(String(err) || "Failed to save settings.");
        return false;
      }
    });

    if (saved === null || saved === false) {
      return false;
    }

    toastContext.actions.showToast(
      "Kiosk settings saved and opened",
      "success"
    );

    return true;
  };

  const handleOpenKiosk = async () => {
    const targetUrl = url.trim() || cfg?.url;

    if (!targetUrl || !isValidUrl(targetUrl)) {
      setLocalError("Enter a valid kiosk URL first.");
      return;
    }

    await gate(async () => {
      if (url.trim() && url.trim() !== cfg?.url) {
        await actions.saveURL(url.trim());
      }
      await actions.saveZoom(zoom);
      await actions.toggleEnabled(true);

      if (isWails) {
        await actions.enterKiosk();
      }
    });

    toastContext.actions.showToast(
      "Kiosk opened",
      "success"
    );
  };

  const handleCloseKiosk = async () => {
    await gate(async () => {
      await actions.toggleEnabled(false);

      if (isWails) {
        await actions.exitKiosk();
      }
    });

    toastContext.actions.showToast(
      "Kiosk closed",
      "success"
    );
  };

  const handleToggleKiosk = async () => {
    if (isKioskCurrentlyActive) {
      await handleCloseKiosk();
    } else {
      await handleOpenKiosk();
    }
  };

  const handleReload = async () => {
    setReloading(true);

    try {
      const res = await gate(async () => {
        await actions.reloadKiosk();
        return true;
      });
      if (res) {
        toastContext.actions.showToast("Kiosk view reloaded", "success");
      }
    } catch {
      toastContext.actions.showToast("Failed to reload kiosk", "danger");
    } finally {
      setTimeout(() => {
        setReloading(false);
      }, 600);
    }
  };

  const cleanup = () => {
    setLocalError(null);
  };

  const getStatusText = () => {
    if (isKioskCurrentlyActive) {
      return "Kiosk mode is currently running.";
    }

    if (!url.trim()) {
      return "Enter a POS web application URL to get started.";
    }

    if (!isUrlValid) {
      return "The URL format is not supported.";
    }

    if (!cfg?.hasPIN) {
      return "Set an admin PIN before enabling kiosk mode.";
    }

    return "Ready to launch kiosk mode.";
  };

  const dialogActions = [
    {
      name: "Cancel",
      label: "Cancel",
      onClick: cleanup,
      variant: "secondary" as ActionType,
    },
    {
      name: "save",
      label: isKioskCurrentlyActive
        ? "Save Changes"
        : "Save & Open Kiosk",
      onClick: saveSettings,
      disabled: !isUrlValid,
      variant: "primary" as ActionType,
    },
  ];

  return (
    <Dialog
      title="Kiosk & Remote Access"
      actions={dialogActions}
      onClose={cleanup}
      openButton={
        <button
          type="button"
          className={`
            w-full flex items-center justify-between
            rounded-xl border px-4 py-3
            transition-colors cursor-pointer
            ${isKioskCurrentlyActive
              ? "border-odoo/40 bg-odoo/5 hover:bg-odoo/10"
              : "border-gray-200 bg-white hover:bg-gray-50"
            }
          `}
        >
          <div className="flex items-center gap-3">
            <div
              className={`
                flex h-9 w-9 items-center justify-center rounded-lg
                ${isKioskCurrentlyActive
                  ? "bg-odoo text-white"
                  : "bg-gray-100 text-gray-500"
                }
              `}
            >
              <svg
                className="h-5 w-5"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <rect
                  x="2"
                  y="3"
                  width="20"
                  height="14"
                  rx="2"
                  strokeWidth="2"
                />
                <line
                  x1="8"
                  y1="21"
                  x2="16"
                  y2="21"
                  strokeWidth="2"
                />
                <line
                  x1="12"
                  y1="17"
                  x2="12"
                  y2="21"
                  strokeWidth="2"
                />
              </svg>
            </div>

            <div className="text-left">
              <div className="text-sm font-semibold text-gray-800">
                Kiosk & Remote Access
              </div>

              <div className="text-xs text-gray-500">
                Configure the kiosk screen and remote access
              </div>
            </div>
          </div>

          <span
            className={`
              rounded-full px-2.5 py-1
              text-[10px] font-semibold uppercase tracking-wide
              ${isKioskCurrentlyActive
                ? "bg-green-100 text-green-700"
                : "bg-gray-100 text-gray-500"
              }
            `}
          >
            {isKioskCurrentlyActive ? "Active" : "Inactive"}
          </span>
        </button>
      }
    >
      <div className="flex flex-col gap-6 text-gray-700">

        {/* ============================================================
            KIOSK MODE
        ============================================================ */}
        <section>
          <div className="mb-3">
            <div className="flex items-center gap-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-odoo/10 text-odoo">
                <svg
                  className="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <rect
                    x="2"
                    y="3"
                    width="20"
                    height="14"
                    rx="2"
                    strokeWidth="2"
                  />
                  <line
                    x1="8"
                    y1="21"
                    x2="16"
                    y2="21"
                    strokeWidth="2"
                  />
                  <line
                    x1="12"
                    y1="17"
                    x2="12"
                    y2="21"
                    strokeWidth="2"
                  />
                </svg>
              </div>

              <div>
                <h3 className="text-sm font-semibold text-gray-900">
                  Kiosk Mode
                </h3>

                <p className="text-xs text-gray-500">
                  Display your POS web application fullscreen.
                </p>
              </div>
            </div>
          </div>

          <div
            className={`
              rounded-xl border p-4
              ${isKioskCurrentlyActive
                ? "border-green-200 bg-green-50/50"
                : "border-gray-200 bg-gray-50/50"
              }
            `}
          >
            {/* Status */}
            <div className="flex items-center justify-between gap-4">
              <div className="flex items-center gap-2.5">
                <span
                  className={`
                    h-2.5 w-2.5 rounded-full
                    ${isKioskCurrentlyActive
                      ? "bg-green-500 animate-pulse"
                      : "bg-gray-300"
                    }
                  `}
                />

                <div>
                  <div className="text-xs font-semibold text-gray-800">
                    {isKioskCurrentlyActive
                      ? "Kiosk is running"
                      : "Kiosk is not running"}
                  </div>

                  <div className="mt-0.5 text-[11px] text-gray-500">
                    {getStatusText()}
                  </div>
                </div>
              </div>

              {isKioskCurrentlyActive && (
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={handleReload}
                    title="Reload kiosk"
                    className="
                      inline-flex items-center gap-1.5
                      rounded-lg border border-gray-300
                      bg-white px-2.5 py-1.5
                      text-xs font-medium text-gray-600
                      shadow-xs transition-colors
                      hover:border-gray-400 hover:bg-gray-50
                      cursor-pointer
                    "
                  >
                    <svg
                      className={`h-3.5 w-3.5 ${reloading ? "animate-spin" : ""
                        }`}
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth="2"
                        d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                      />
                    </svg>

                    {reloading ? "Reloading" : "Reload"}
                  </button>
                </div>
              )}
            </div>

            {/* URL */}
            <div className="mt-4">
              <label
                htmlFor="kiosk-url"
                className="mb-1.5 block text-xs font-medium text-gray-700"
              >
                POS Web Application URL
              </label>

              <div className="relative">
                <input
                  id="kiosk-url"
                  type="url"
                  value={url}
                  placeholder="https://your-domain.odoo.com/pos-self/204?access_token=..."
                  onChange={(e) => {
                    setUrl(e.target.value);
                    setLocalError(null);
                  }}
                  className={`
                    w-full rounded-lg border
                    bg-white px-3 py-2.5
                    text-sm font-mono
                    outline-none transition
                    ${localError
                      ? "border-red-300 focus:border-red-400"
                      : "border-gray-300 focus:border-odoo"
                    }
                  `}
                />
              </div>

              <p className="mt-1.5 text-[11px] text-gray-500">
                Use the URL of your Odoo POS Self Order / Kiosk page.
              </p>
            </div>

            {/* Display Zoom */}
            <div className="mt-4 border-t border-gray-200 pt-3">
              <div className="flex items-center justify-between mb-2">
                <div>
                  <label
                    htmlFor="kiosk-zoom"
                    className="block text-xs font-medium text-gray-700"
                  >
                    Iframe Display Zoom
                  </label>
                  <p className="text-[11px] text-gray-500">
                    Scale the POS interface up or down to fit your display.
                  </p>
                </div>
                <span className="rounded-full bg-odoo/10 px-2.5 py-0.5 text-xs font-bold text-odoo">
                  {Math.round(zoom * 100)}%
                </span>
              </div>

              {/* Stepper + Slider */}
              <div className="flex items-center gap-3">
                <button
                  type="button"
                  onClick={() => handleZoomChange(zoom - 0.05)}
                  disabled={zoom <= 0.5}
                  title="Zoom out"
                  className="
                    flex h-8 w-8 shrink-0 items-center justify-center
                    rounded-lg border border-gray-300 bg-white
                    text-gray-600 shadow-xs transition-colors
                    hover:border-gray-400 hover:bg-gray-50
                    disabled:opacity-40 disabled:cursor-not-allowed
                    cursor-pointer
                  "
                >
                  <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <line x1="5" y1="12" x2="19" y2="12" strokeWidth="2" strokeLinecap="round" />
                  </svg>
                </button>

                <input
                  id="kiosk-zoom"
                  type="range"
                  min="0.5"
                  max="2.0"
                  step="0.05"
                  value={zoom}
                  onChange={(e) => handleZoomChange(parseFloat(e.target.value))}
                  className="w-full accent-odoo cursor-pointer h-2 bg-gray-200 rounded-lg"
                />

                <button
                  type="button"
                  onClick={() => handleZoomChange(zoom + 0.05)}
                  disabled={zoom >= 2.0}
                  title="Zoom in"
                  className="
                    flex h-8 w-8 shrink-0 items-center justify-center
                    rounded-lg border border-gray-300 bg-white
                    text-gray-600 shadow-xs transition-colors
                    hover:border-gray-400 hover:bg-gray-50
                    disabled:opacity-40 disabled:cursor-not-allowed
                    cursor-pointer
                  "
                >
                  <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <line x1="12" y1="5" x2="12" y2="19" strokeWidth="2" strokeLinecap="round" />
                    <line x1="5" y1="12" x2="19" y2="12" strokeWidth="2" strokeLinecap="round" />
                  </svg>
                </button>
              </div>

              {/* Preset buttons */}
              <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
                {[0.75, 0.9, 1.0, 1.1, 1.25, 1.5].map((preset) => {
                  const isSelected = Math.abs(zoom - preset) < 0.01;
                  return (
                    <button
                      key={preset}
                      type="button"
                      onClick={() => handleZoomChange(preset)}
                      className={`
                        rounded-md px-2 py-0.5 text-[11px] font-medium transition-colors cursor-pointer
                        ${isSelected
                          ? "bg-odoo text-white shadow-xs"
                          : "border border-gray-200 bg-white text-gray-600 hover:bg-gray-50"
                        }
                      `}
                    >
                      {Math.round(preset * 100)}%
                    </button>
                  );
                })}
                {Math.abs(zoom - 1.0) >= 0.01 && (
                  <button
                    type="button"
                    onClick={() => handleZoomChange(1.0)}
                    className="ml-auto text-[11px] font-medium text-odoo hover:underline cursor-pointer"
                  >
                    Reset (100%)
                  </button>
                )}
              </div>
            </div>

            {/* Inline error */}
            {localError && (
              <div className="mt-3 flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2.5 text-xs text-red-700">
                <svg
                  className="mt-0.5 h-4 w-4 shrink-0"
                  fill="currentColor"
                  viewBox="0 0 20 20"
                >
                  <path
                    fillRule="evenodd"
                    d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z"
                    clipRule="evenodd"
                  />
                </svg>

                <span>{localError}</span>
              </div>
            )}

            {/* PIN warning */}
            {!cfg?.hasPIN && (
              <div className="mt-3 flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2.5 text-xs text-amber-800">
                <svg
                  className="mt-0.5 h-4 w-4 shrink-0"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth="2"
                    d="M12 9v3m0 4h.01M10.29 3.86l-8.1 14a2 2 0 001.73 3h16.16a2 2 0 001.73-3l-8.1-14a2 2 0 00-3.42 0z"
                  />
                </svg>

                <div>
                  <div className="font-medium">
                    Admin PIN required
                  </div>
                  <div className="mt-0.5 text-amber-700">
                    Set a PIN in the App settings before enabling kiosk mode.
                  </div>
                </div>
              </div>
            )}

            {/* Toggle */}
            <div className="mt-4 flex items-center justify-between border-t border-gray-200 pt-3">
              <div>
                <div className="text-xs font-medium text-gray-800">
                  Enable kiosk mode
                </div>

                <div className="text-[11px] text-gray-500">
                  Open the configured page in fullscreen.
                </div>
              </div>

              <button
                type="button"
                disabled={!canEnable}
                onClick={handleToggleKiosk}
                aria-label={
                  isKioskCurrentlyActive
                    ? "Disable kiosk mode"
                    : "Enable kiosk mode"
                }
                className={`
                  relative h-6 w-11 shrink-0 rounded-full
                  transition-colors
                  ${isKioskCurrentlyActive
                    ? "bg-odoo"
                    : "bg-gray-300"
                  }
                  ${canEnable
                    ? "cursor-pointer"
                    : "cursor-not-allowed opacity-40"
                  }
                `}
              >
                <span
                  className={`
                    absolute top-1 h-4 w-4
                    rounded-full bg-white shadow-sm
                    transition-transform
                    ${isKioskCurrentlyActive
                      ? "left-6"
                      : "left-1"
                    }
                  `}
                />
              </button>
            </div>

            {/* Default Launcher — Android only */}
            {isWails && (
              <div className="mt-3 flex items-center justify-between border-t border-gray-200 pt-3">
                <div>
                  <div className="text-xs font-medium text-gray-800">
                    Default launcher app
                  </div>

                  <div className="text-[11px] text-gray-500">
                    Set this app as the Android home screen launcher.
                  </div>
                </div>

                <button
                  id="set-default-launcher-btn"
                  type="button"
                  onClick={setDefaultLauncher}
                  className="
                    inline-flex shrink-0 items-center gap-1.5
                    rounded-lg border border-gray-300
                    bg-white px-3 py-1.5
                    text-xs font-medium text-gray-700
                    shadow-xs transition-colors
                    hover:border-odoo hover:bg-odoo/5 hover:text-odoo
                    cursor-pointer
                  "
                >
                  <svg
                    className="h-3.5 w-3.5"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth="2"
                      d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
                    />
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth="2"
                      d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                    />
                  </svg>
                  Open Settings
                </button>
              </div>
            )}
          </div>
        </section>


        {/* ============================================================
            REMOTE ACCESS
        ============================================================ */}
        {isWails && <section>
          <div className="mb-3">
            <div className="flex items-center gap-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-50 text-blue-600">
                <svg
                  className="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth="2"
                    d="M12 18h.01M8 21h8a2 2 0 002-2V5a2 2 0 00-2-2H8a2 2 0 00-2 2v14a2 2 0 002 2z"
                  />
                </svg>
              </div>

              <div>
                <h3 className="text-sm font-semibold text-gray-900">
                  Remote Access
                </h3>

                <p className="text-xs text-gray-500">
                  Open ePOS Proxy from another device on the same network.
                </p>
              </div>
            </div>
          </div>

          <div className="rounded-xl border border-gray-200 bg-gray-50/50 p-4">
            <div className="flex flex-col sm:flex-row items-center sm:items-start gap-4">
              {/* QR */}
              <div className="shrink-0 rounded-xl border border-gray-200 bg-white p-2.5 shadow-xs">
                <QRCodeSVG
                  value={serverUrl}
                  size={120}
                  level="M"
                />
              </div>

              {/* Info text */}
              <div className="min-w-0 flex-1 text-center sm:text-left">
                <div className="text-xs font-semibold text-gray-800">
                  Scan to connect
                </div>

                <p className="mt-1 text-[11px] leading-relaxed text-gray-500">
                  Scan this QR code with a phone, tablet, or another
                  computer connected to the same network to access the ePOS proxy.
                </p>

                <div className="mt-2.5 inline-flex items-center gap-1.5 rounded-md bg-blue-50 px-2.5 py-1 text-[10px] font-medium text-blue-700">
                  <span className="h-1.5 w-1.5 rounded-full bg-blue-500 animate-pulse" />
                  Local Network Address
                </div>
              </div>
            </div>

          </div>
          {/* Full-width URL display */}
          <div className="mt-3.5 border-t border-gray-200/80 pt-3">
            <label className="mb-1 block text-[11px] font-medium text-gray-600">
              Direct Connection URL
            </label>
            <div className="flex items-center gap-2 rounded-lg border border-gray-300 bg-white p-1.5 shadow-xs">
              <span className="min-w-0 flex-1 px-2 py-1 text-xs font-mono text-gray-800 select-all break-all">
                {serverUrl}
              </span>

              <button
                type="button"
                onClick={() => copyServerUrl(serverUrl)}
                className={`
                    inline-flex shrink-0 items-center gap-1.5
                    rounded-md px-3 py-1.5
                    text-xs font-medium text-white
                    shadow-xs transition-colors cursor-pointer
                    ${copiedUrl ? "bg-green-600 hover:bg-green-700" : "bg-odoo hover:bg-odoo-dark"}
                  `}
              >
                {copiedUrl ? (
                  <>
                    <svg
                      className="h-3.5 w-3.5"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth="2"
                        d="M5 13l4 4L19 7"
                      />
                    </svg>
                    Copied!
                  </>
                ) : (
                  <>
                    <svg
                      className="h-3.5 w-3.5"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth="2"
                        d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                      />
                    </svg>
                  </>
                )}
              </button>
            </div>
          </div>
        </section>}

        {/* ============================================================
            SECURITY NOTE
        ============================================================ */}
        <div className="flex items-start gap-2.5 border-t border-gray-200 pt-4">
          <svg
            className="mt-0.5 h-4 w-4 shrink-0 text-red-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth="2"
              d="M12 9v3m0 4h.01M12 3a9 9 0 100 18 9 9 0 000-18z"
            />
          </svg>

          <p className="text-[11px] leading-relaxed text-red-500">
            {isWails
              ? "To exit fullscreen kiosk mode, tap the top-right corner 4 times quickly and enter your admin PIN."
              : "Kiosk fullscreen mode runs on the desktop application. This web interface is used to manage the kiosk and remote access settings."}
          </p>
        </div>
      </div>
    </Dialog>
  );
}
