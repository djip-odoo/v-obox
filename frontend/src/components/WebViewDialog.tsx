import { useContext, useEffect, useRef, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import { WebViewContext } from "../contexts/WebViewContext";
import { AppContext } from "../contexts/AppContext";
import { PINContext } from "../contexts/PINContext";
import { ToastContext } from "../contexts/ToastContext";
import Dialog from "./Dialog";
import { useClipboard } from "../hooks/useClipboard";
import { backendService } from "../services/backend";

export function isValidUrl(urlStr: string): boolean {
  const trimmed = urlStr.trim();

  if (!trimmed) {
    return false;
  }

  try {
    const parsed = new URL(trimmed);

    if (parsed.protocol !== "https:" && parsed.protocol !== "http:") {
      return false;
    }

    return Boolean(parsed.hostname);
  } catch {
    return false;
  }
}

export default function WebViewDialog() {
  const { data: { isWails, isAndroid } } = useContext(AppContext);
  const toastContext = useContext(ToastContext);
  const { data, actions } = useContext(WebViewContext);
  const { showPINDialog } = useContext(PINContext);
  const { setDefaultLauncher } = actions;
  const cfg = data.config;

  const [url, setUrl] = useState(cfg?.url ?? "");
  const [zoom, setZoom] = useState<number>(cfg?.zoom && cfg.zoom > 0 ? cfg.zoom : 1.0);
  const [localError, setLocalError] = useState<string | null>(null);
  const [serverUrl, setServerUrl] = useState(window.location.origin + "/");
  const [reloading, setReloading] = useState(false);
  const [saveStatus, setSaveStatus] = useState<"idle" | "saving" | "saved">("idle");

  const saveTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastSavedUrlRef = useRef<string>(cfg?.url ?? "");

  const { copied: copiedUrl, copy: copyServerUrl } = useClipboard({
    successMessage: "Server URL copied to clipboard",
    errorMessage: "Failed to copy URL",
  });

  const isUrlValid = isValidUrl(url);
  const isKioskCurrentlyActive = Boolean(cfg?.isActive);
  const canEnable = Boolean(isUrlValid && cfg?.hasPIN);

  /*
   * Keep URL and Zoom synchronized with remote or Go backend updates.
   */
  useEffect(() => {
    if (cfg?.url !== undefined && cfg.url !== lastSavedUrlRef.current) {
      setUrl(cfg.url);
      lastSavedUrlRef.current = cfg.url;
    }
    if (cfg?.zoom && cfg.zoom > 0) {
      setZoom(cfg.zoom);
    }
  }, [cfg?.url, cfg?.zoom]);

  /*
   * Fetch troubleshoot info for direct local network IP access.
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

  /*
   * Auto-save URL with debounce when valid.
   */
  const triggerAutoSaveUrl = (newUrl: string) => {
    const trimmed = newUrl.trim();
    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current);
    }

    if (trimmed === lastSavedUrlRef.current) {
      return;
    }

    if (!trimmed) {
      setLocalError(null);
      saveTimeoutRef.current = setTimeout(async () => {
        try {
          setSaveStatus("saving");
          await actions.saveURL("");
          lastSavedUrlRef.current = "";
          setSaveStatus("saved");
          setTimeout(() => setSaveStatus("idle"), 1500);
        } catch {
          setSaveStatus("idle");
        }
      }, 500);
      return;
    }

    if (!isValidUrl(trimmed)) {
      setLocalError("Enter a valid URL (must start with http:// or https://).");
      return;
    }

    setLocalError(null);
    setSaveStatus("saving");
    saveTimeoutRef.current = setTimeout(async () => {
      try {
        await actions.saveURL(trimmed);
        lastSavedUrlRef.current = trimmed;
        setSaveStatus("saved");
        setTimeout(() => setSaveStatus("idle"), 1500);
      } catch (err: unknown) {
        setLocalError(String(err) || "Failed to save URL.");
        setSaveStatus("idle");
      }
    }, 600);
  };

  const handleUrlChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const nextVal = e.target.value;
    setUrl(nextVal);
    triggerAutoSaveUrl(nextVal);
  };

  const handleUrlBlur = async () => {
    const trimmed = url.trim();
    if (trimmed && isValidUrl(trimmed) && trimmed !== lastSavedUrlRef.current) {
      if (saveTimeoutRef.current) clearTimeout(saveTimeoutRef.current);
      try {
        setSaveStatus("saving");
        await actions.saveURL(trimmed);
        lastSavedUrlRef.current = trimmed;
        setSaveStatus("saved");
        setTimeout(() => setSaveStatus("idle"), 1500);
      } catch (err: unknown) {
        setLocalError(String(err) || "Failed to save URL.");
        setSaveStatus("idle");
      }
    }
  };

  const handleZoomChange = async (newZoom: number) => {
    const clamped = Math.round(Math.min(2.0, Math.max(0.5, newZoom)) * 100) / 100;
    setZoom(clamped);
    try {
      await actions.saveZoom(clamped);
    } catch (err) {
      console.error("Failed to save zoom:", err);
    }
  };

  const handleLaunchWebapp = async () => {
    setLocalError(null);
    const targetUrl = url.trim() || cfg?.url || "";

    if (!targetUrl || !isValidUrl(targetUrl)) {
      setLocalError("Enter a valid URL (must start with http:// or https://).");
      return;
    }

    try {
      if (targetUrl !== lastSavedUrlRef.current) {
        await actions.saveURL(targetUrl);
        lastSavedUrlRef.current = targetUrl;
      }
      await actions.saveZoom(zoom);
      await actions.openWebapp(targetUrl);

      toastContext.actions.showToast("Opening Web App...", "success");
    } catch (err: unknown) {
      setLocalError(String(err) || "Failed to launch Web App.");
    }
  };

  const handleCloseWebapp = async () => {
    if (cfg?.hasPIN) {
      const pinVerified = await showPINDialog();
      if (!pinVerified) {
        return;
      }
    }

    try {
      await actions.closeWebapp();
      toastContext.actions.showToast("Returned to Proxy Settings", "success");
    } catch (err: unknown) {
      setLocalError(String(err) || "Failed to close Web App.");
    }
  };

  const handleToggleLockdown = async () => {
    setLocalError(null);
    const newEnabled = !cfg?.enabled;

    if (newEnabled && !cfg?.hasPIN) {
      setLocalError("Set an admin PIN before enabling lockdown mode.");
      return;
    }

    const pinVerified = await showPINDialog();
    if (!pinVerified) {
      return;
    }

    try {
      if (url.trim() && isValidUrl(url.trim()) && url.trim() !== lastSavedUrlRef.current) {
        await actions.saveURL(url.trim());
        lastSavedUrlRef.current = url.trim();
      }
      await actions.toggleEnabled(newEnabled);

      toastContext.actions.showToast(
        newEnabled ? "Lockdown mode enabled" : "Lockdown mode disabled",
        "success"
      );
    } catch (err: unknown) {
      setLocalError(String(err) || "Failed to toggle lockdown mode.");
    }
  };

  const handleReload = async () => {
    setReloading(true);
    try {
      await actions.reloadKiosk();
      toastContext.actions.showToast("Web App reloaded", "success");
    } catch {
      toastContext.actions.showToast("Failed to reload Web App", "danger");
    } finally {
      setTimeout(() => {
        setReloading(false);
      }, 600);
    }
  };

  const cleanup = () => {
    setLocalError(null);
  };

  return (
    <Dialog
      title="Kiosk & Remote Access"
      onClose={cleanup}
      showTitleDivider
      openButton={
        <button
          type="button"
          className={`
            w-full flex items-center justify-between
            rounded-xl border px-4 py-3.5
            transition-all duration-150 cursor-pointer text-left
            ${isKioskCurrentlyActive
              ? "border-odoo/40 bg-odoo/5 shadow-xs hover:bg-odoo/10"
              : "border-gray-200 bg-white hover:border-gray-300 hover:bg-gray-50/80 shadow-xs"
            }
          `}
        >
          <div className="flex items-center gap-3.5">
            <div
              className={`
                flex h-10 w-10 items-center justify-center rounded-xl transition-colors
                ${isKioskCurrentlyActive
                  ? "bg-odoo text-white shadow-xs"
                  : "bg-gray-100 text-gray-600"
                }
              `}
            >
              <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <rect x="2" y="3" width="20" height="14" rx="2" strokeWidth="2" />
                <line x1="8" y1="21" x2="16" y2="21" strokeWidth="2" />
                <line x1="12" y1="17" x2="12" y2="21" strokeWidth="2" />
              </svg>
            </div>

            <div>
              <div className="text-sm font-semibold text-gray-900">
                Kiosk & Remote Access
              </div>
              <div className="text-xs text-gray-500">
                {isKioskCurrentlyActive
                  ? "Web App is currently active"
                  : "Configure POS Web App, Zoom, & Remote Access"}
              </div>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <span
              className={`
                inline-flex items-center gap-1.5 rounded-full px-2.5 py-1
                text-[11px] font-semibold tracking-wide
                ${isKioskCurrentlyActive
                  ? "bg-emerald-100 text-emerald-800"
                  : "bg-gray-100 text-gray-600"
                }
              `}
            >
              <span
                className={`h-1.5 w-1.5 rounded-full ${isKioskCurrentlyActive ? "bg-emerald-500 animate-pulse" : "bg-gray-400"
                  }`}
              />
              {isKioskCurrentlyActive ? "Active" : "Standby"}
            </span>
          </div>
        </button>
      }
    >
      <div className="flex flex-col gap-5 py-1 text-gray-700">

        {/* ============================================================
            KIOSK / WEB APP SECTION
        ============================================================ */}
        <section className="rounded-xl border border-gray-200 bg-white p-4 shadow-xs">
          {/* Header Row with Status */}
          <div className="flex items-center justify-between pb-3 border-b border-gray-100">
            <div className="flex items-center gap-2.5">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-odoo/10 text-odoo">
                <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                </svg>
              </div>
              <div>
                <h3 className="text-xs font-bold uppercase tracking-wider text-gray-700">
                  POS Web Application
                </h3>
                <p className="text-[11px] text-gray-500">
                  {cfg?.isActive
                    ? "Currently running on the main display."
                    : cfg?.enabled
                      ? "Configured to run in Fullscreen Lockdown mode."
                      : "Ready to launch in standard windowed mode."}
                </p>
              </div>
            </div>

            {/* Auto-save Status Indicator */}
            <div className="flex items-center gap-1.5 text-[11px] text-gray-500">
              {saveStatus === "saving" && (
                <span className="inline-flex items-center gap-1 text-amber-600">
                  <span className="h-1.5 w-1.5 rounded-full bg-amber-500 animate-ping" />
                  Saving...
                </span>
              )}
              {saveStatus === "saved" && (
                <span className="inline-flex items-center gap-1 text-emerald-600 font-medium">
                  <svg className="h-3.5 w-3.5 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2.5" d="M5 13l4 4L19 7" />
                  </svg>
                  Saved
                </span>
              )}
            </div>
          </div>

          {/* URL Input Form */}
          <div className="mt-3.5">
            <div className="flex items-center justify-between mb-1.5">
              <label htmlFor="kiosk-url" className="block text-xs font-semibold text-gray-800">
                Application URL
              </label>
              <span className="text-[10px] text-gray-400">Auto-saved</span>
            </div>

            <div className="relative flex items-center">
              <input
                id="kiosk-url"
                type="url"
                value={url}
                placeholder="https://your-domain.odoo.com/pos-self/204?access_token=..."
                onChange={handleUrlChange}
                onBlur={handleUrlBlur}
                className={`
                  w-full rounded-lg border bg-gray-50/50 px-3 py-2 text-xs font-mono
                  outline-none transition-all duration-150
                  ${localError
                    ? "border-red-300 focus:border-red-500 focus:bg-white focus:ring-1 focus:ring-red-200"
                    : "border-gray-300 focus:border-odoo focus:bg-white focus:ring-2 focus:ring-odoo/10"
                  }
                `}
              />
            </div>

            {localError ? (
              <div className="mt-2 flex items-center gap-1.5 text-xs text-red-600">
                <svg className="h-3.5 w-3.5 shrink-0" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
                </svg>
                <span>{localError}</span>
              </div>
            ) : (
              <p className="mt-1.5 text-[11px] text-gray-500">
                Enter your Odoo POS URL (HTTP or HTTPS). Changes persist automatically.
              </p>
            )}
          </div>

          {/* Action Toolbar */}
          <div className="mt-4 flex items-center justify-between gap-2.5 rounded-lg bg-gray-50 p-2.5 border border-gray-200/70">
            <div className="flex items-center gap-2">
              <span className={`h-2 w-2 rounded-full ${isKioskCurrentlyActive ? "bg-emerald-500 animate-pulse" : "bg-gray-400"}`} />
              <span className="text-xs font-medium text-gray-700">
                {isKioskCurrentlyActive ? "Web App is Active" : "Web App Inactive"}
              </span>
            </div>

            <div className="flex items-center gap-2">
              {/* Reload Button */}
              <button
                type="button"
                onClick={handleReload}
                disabled={reloading || (!isKioskCurrentlyActive && !isUrlValid)}
                title="Reload Web App"
                className="
                  inline-flex items-center gap-1.5 rounded-lg border border-gray-300 bg-white
                  px-2.5 py-1.5 text-xs font-medium text-gray-700 shadow-xs
                  hover:bg-gray-50 transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed
                "
              >
                <svg
                  className={`h-3.5 w-3.5 text-gray-600 ${reloading ? "animate-spin text-odoo" : ""}`}
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                </svg>
                <span>Reload</span>
              </button>

              {/* Close or Launch Button */}
              {isKioskCurrentlyActive ? (
                <button
                  type="button"
                  onClick={handleCloseWebapp}
                  className="
                    inline-flex items-center gap-1.5 rounded-lg bg-red-600
                    px-3 py-1.5 text-xs font-medium text-white shadow-xs
                    hover:bg-red-700 transition-colors cursor-pointer
                  "
                >
                  <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M6 18L18 6M6 6l12 12" />
                  </svg>
                  <span>Close & Exit</span>
                </button>
              ) : (
                <button
                  type="button"
                  disabled={!isUrlValid}
                  onClick={handleLaunchWebapp}
                  className={`
                    inline-flex items-center gap-1.5 rounded-lg px-3.5 py-1.5 text-xs font-medium text-white shadow-xs transition-colors
                    ${isUrlValid
                      ? "bg-odoo hover:bg-odoo-dark cursor-pointer"
                      : "bg-gray-300 cursor-not-allowed opacity-60"
                    }
                  `}
                >
                  <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                  <span>Launch POS</span>
                </button>
              )}
            </div>
          </div>
        </section>

        {/* ============================================================
            DISPLAY ZOOM / SCALING SECTION
        ============================================================ */}
        <section className="rounded-xl border border-gray-200 bg-white p-4 shadow-xs">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-2">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-purple-50 text-purple-600">
                <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0zM10 7v3m0 0v3m0-3h3m-3 0H7" />
                </svg>
              </div>
              <div>
                <h3 className="text-xs font-bold uppercase tracking-wider text-gray-700">
                  Display Scaling
                </h3>
                <p className="text-[11px] text-gray-500">
                  Adjust UI scale for your monitor resolution.
                </p>
              </div>
            </div>

            <span className="rounded-md bg-odoo/10 px-2.5 py-1 text-xs font-bold text-odoo">
              {Math.round(zoom * 100)}%
            </span>
          </div>

          {/* Stepper + Slider Control */}
          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={() => handleZoomChange(zoom - 0.05)}
              disabled={zoom <= 0.5}
              title="Zoom out"
              className="
                flex h-8 w-8 shrink-0 items-center justify-center
                rounded-lg border border-gray-300 bg-white
                text-gray-700 shadow-xs transition-colors
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
                text-gray-700 shadow-xs transition-colors
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

          {/* Quick Preset Buttons */}
          <div className="mt-3 flex flex-wrap items-center gap-1.5">
            {[0.75, 0.9, 1.0, 1.1, 1.25, 1.5].map((preset) => {
              const isSelected = Math.abs(zoom - preset) < 0.01;
              return (
                <button
                  key={preset}
                  type="button"
                  onClick={() => handleZoomChange(preset)}
                  className={`
                    rounded-md px-2.5 py-1 text-xs font-medium transition-colors cursor-pointer
                    ${isSelected
                      ? "bg-odoo text-white shadow-xs"
                      : "border border-gray-200 bg-gray-50 text-gray-700 hover:bg-gray-100"
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
                className="ml-auto text-xs font-semibold text-odoo hover:underline cursor-pointer"
              >
                Reset (100%)
              </button>
            )}
          </div>
        </section>

        {/* ============================================================
            LOCKDOWN & SECURITY TOGGLE
        ============================================================ */}
        {isAndroid && <section className="rounded-xl border border-gray-200 bg-white p-4 shadow-xs">
          <div className="flex items-center justify-between">
            <div className="pr-4">
              <div className="flex items-center gap-2">
                <h3 className="text-xs font-bold uppercase tracking-wider text-gray-700">
                  Lockdown Mode (Fullscreen Kiosk)
                </h3>
              </div>
              <p className="mt-0.5 text-xs text-gray-500 leading-relaxed">
                When active, the POS opens in full screen and prevents accidental exits. Requires admin PIN to leave.
              </p>
            </div>

            <button
              type="button"
              disabled={!canEnable}
              onClick={handleToggleLockdown}
              aria-label={cfg?.enabled ? "Disable lockdown mode" : "Enable lockdown mode"}
              className={`
                relative h-6 w-11 shrink-0 rounded-full transition-colors
                ${cfg?.enabled ? "bg-odoo" : "bg-gray-300"}
                ${canEnable ? "cursor-pointer" : "cursor-not-allowed opacity-40"}
              `}
            >
              <span
                className={`
                  absolute top-1 h-4 w-4 rounded-full bg-white shadow-sm transition-transform
                  ${cfg?.enabled ? "left-6" : "left-1"}
                `}
              />
            </button>
          </div>

          {!cfg?.hasPIN && (
            <div className="mt-3 flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-xs text-amber-800">
              <svg className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 9v3m0 4h.01M10.29 3.86l-8.1 14a2 2 0 001.73 3h16.16a2 2 0 001.73-3l-8.1-14a2 2 0 00-3.42 0z" />
              </svg>
              <div>
                <span className="font-semibold">Admin PIN required:</span> Set a PIN in the App menu before enabling lockdown mode.
              </div>
            </div>
          )}

          {/* Android Default Launcher Button if supported */}
          {isWails && backendService.isAndroidLauncherSupported?.() && (
            <div className="mt-3 pt-3 border-t border-gray-100 flex items-center justify-between">
              <div>
                <div className="text-xs font-semibold text-gray-800">Default Home Launcher</div>
                <div className="text-[11px] text-gray-500">Set this proxy as the default Android home app.</div>
              </div>
              <button
                id="set-default-launcher-btn"
                type="button"
                onClick={setDefaultLauncher}
                className="rounded-lg border border-gray-300 bg-white px-3 py-1.5 text-xs font-medium text-gray-700 shadow-xs hover:bg-gray-50 cursor-pointer"
              >
                Set Launcher
              </button>
            </div>
          )}
        </section>}

        {/* ============================================================
            REMOTE ACCESS SECTION (QR + IP)
        ============================================================ */}
        {isWails && (
          <section className="rounded-xl border border-gray-200 bg-white p-4 shadow-xs">
            <div className="flex items-center gap-2 mb-3">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-blue-50 text-blue-600">
                <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 18h.01M8 21h8a2 2 0 002-2V5a2 2 0 00-2-2H8a2 2 0 00-2 2v14a2 2 0 002 2z" />
                </svg>
              </div>
              <div>
                <h3 className="text-xs font-bold uppercase tracking-wider text-gray-700">
                  Remote Access & QR
                </h3>
                <p className="text-[11px] text-gray-500">
                  Manage this proxy from another phone or PC on the same Wi-Fi.
                </p>
              </div>
            </div>

            <div className="flex flex-col sm:flex-row items-center gap-4 bg-gray-50/70 p-3.5 rounded-xl border border-gray-200/80">
              <div className="shrink-0 rounded-lg border border-gray-200 bg-white p-2 shadow-xs">
                <QRCodeSVG value={serverUrl} size={105} level="M" />
              </div>
            </div>
            <div className="min-w-0 flex-1 text-center sm:text-left">
              <div className="text-xs font-bold text-gray-800">Direct Network Address</div>
              <div className="mt-1.5 flex items-center gap-2 rounded-lg border border-gray-300 bg-white p-1.5 shadow-xs">
                <span className="min-w-0 flex-1 px-2 py-0.5 text-xs font-mono text-gray-800 select-all break-all">
                  {serverUrl}
                </span>
                <button
                  type="button"
                  onClick={() => copyServerUrl(serverUrl)}
                  className={`
                      inline-flex shrink-0 items-center gap-1 rounded-md px-2.5 py-1 text-xs font-medium text-white shadow-xs transition-colors cursor-pointer
                      ${copiedUrl ? "bg-emerald-600 hover:bg-emerald-700" : "bg-odoo hover:bg-odoo-dark"}
                    `}
                >
                  {copiedUrl ? "Copied!" : "Copy"}
                </button>
              </div>
            </div>
          </section>
        )}

        {/* ============================================================
            SECURITY / GESTURE NOTICE
        ============================================================ */}
        {
          isAndroid &&
          <div className="flex items-start gap-2.5 rounded-xl bg-slate-50 border border-slate-200/80 p-3 text-xs text-slate-600">
            <svg className="mt-0.5 h-4 w-4 shrink-0 text-slate-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <p className="text-[11px] leading-relaxed">
              {isWails
                ? "To exit fullscreen kiosk mode while running the POS, quickly tap any screen corner 4 times and enter your admin PIN."
                : "Kiosk fullscreen mode runs directly on the local display. You can control and configure settings remotely from this interface."}
            </p>
          </div>
        }

      </div>
    </Dialog>
  );
}
