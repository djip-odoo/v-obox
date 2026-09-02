import { useCallback, useContext, useState } from "react";
import Dialog, { ActionType } from "./Dialog";
import { AppContext } from "../contexts/AppContext";
import { backendService } from "../services/backend";
import { DeviceInfo } from "../types/models";
import { useClipboard } from "../hooks/useClipboard";

function formatBytes(bytes?: number): string | undefined {
  if (bytes === undefined || bytes === null || bytes <= 0) return undefined;
  const gb = bytes / (1024 * 1024 * 1024);
  if (gb >= 1) return `${gb.toFixed(1)} GB`;
  const mb = bytes / (1024 * 1024);
  return `${mb.toFixed(0)} MB`;
}

function formatUptime(ms?: number): string | undefined {
  if (ms === undefined || ms === null || ms <= 0) return undefined;
  const totalSeconds = Math.floor(ms / 1000);
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const parts: string[] = [];
  if (days > 0) parts.push(`${days}d`);
  if (hours > 0) parts.push(`${hours}h`);
  parts.push(`${minutes}m`);
  return parts.length > 0 ? parts.join(" ") : undefined;
}

function getChromiumMajorVersion(versionStr?: string, ua?: string): number | null {
  if (versionStr) {
    const match = versionStr.match(/^(\d+)/);
    if (match) return parseInt(match[1], 10);
  }
  if (ua) {
    const match = ua.match(/Chrome\/(\d+)/);
    if (match) return parseInt(match[1], 10);
  }
  return null;
}

export default function DeviceInfoDialog() {
  const { data: { isWails } } = useContext(AppContext);
  const [info, setInfo] = useState<DeviceInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const { copy: copyDiagnostics } = useClipboard({
    successMessage: "Device diagnostics copied to clipboard",
    errorMessage: "Failed to copy diagnostics",
  });

  const { copy: copyUserAgent } = useClipboard({
    successMessage: "User-Agent copied to clipboard",
    errorMessage: "Failed to copy User-Agent",
  });

  const fetchDeviceInfo = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await backendService.getDeviceInfo();
      setInfo(data);
    } catch (err: unknown) {
      setError(String(err) || "Failed to load device info.");
    } finally {
      setLoading(false);
    }
  }, []);

  const handleOpen = () => {
    fetchDeviceInfo();
  };

  const handleCopyAll = () => {
    if (!info) return;
    const summary = JSON.stringify(info, null, 2);
    copyDiagnostics(summary);
  };

  const ramUsedBytes = info?.usedRamBytes ?? (info?.totalRamBytes !== undefined && info?.availableRamBytes !== undefined ? info.totalRamBytes - info.availableRamBytes : undefined);
  const ramPercent = (ramUsedBytes !== undefined && info?.totalRamBytes !== undefined && info.totalRamBytes > 0)
    ? Math.min(100, Math.round((ramUsedBytes / info.totalRamBytes) * 100))
    : undefined;

  const storageUsedBytes = info?.usedStorageBytes ?? (info?.totalStorageBytes !== undefined && info?.availableStorageBytes !== undefined ? info.totalStorageBytes - info.availableStorageBytes : undefined);
  const storagePercent = (storageUsedBytes !== undefined && info?.totalStorageBytes !== undefined && info.totalStorageBytes > 0)
    ? Math.min(100, Math.round((storageUsedBytes / info.totalStorageBytes) * 100))
    : undefined;

  const chromeMajor = getChromiumMajorVersion(info?.webViewVersion, info?.userAgent);
  const isModernChromium = chromeMajor !== null ? chromeMajor >= 120 : undefined;

  return (
    <Dialog
      title="Device & System Information"
      onOpen={handleOpen}
      actions={[
        {
          name: "refresh",
          label: loading ? "Refreshing..." : "Refresh",
          onClick: () => {
            fetchDeviceInfo();
            return false;
          },
          variant: "secondary" as ActionType,
          disabled: loading,
        },
        {
          name: "copy",
          label: "Copy Diagnostics",
          onClick: () => {
            handleCopyAll();
            return false;
          },
          variant: "primary" as ActionType,
          disabled: !info,
        },
      ]}
      openButton={
        <button
          type="button"
          id="device-info-btn"
          className="
            w-full flex items-center justify-center gap-2 py-3 px-4
            rounded-xl border border-gray-200 bg-white hover:bg-gray-50/90
            text-gray-700 font-medium text-sm transition-all duration-200
            shadow-xs active:scale-[0.99] cursor-pointer
          "
        >
          <svg
            className="w-4 h-4 text-gray-500"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth="2"
              d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"
            />
          </svg>
          <span>Device Info & Diagnostics</span>
        </button>
      }
    >
      {loading && !info ? (
        <div className="flex flex-col items-center justify-center py-10 gap-3">
          <div className="w-8 h-8 border-3 border-odoo border-t-transparent rounded-full animate-spin" />
          <span className="text-xs text-gray-500">Querying actual device hardware info...</span>
        </div>
      ) : error ? (
        <div className="bg-red-50 border border-red-200 text-red-700 rounded-lg p-4 text-xs">
          {error}
        </div>
      ) : info ? (
        <div className="space-y-4 max-h-[70vh] overflow-y-auto pr-1 text-xs">

          {!isWails && (
            <div className="flex items-center gap-2 p-2.5 rounded-lg bg-blue-50/80 border border-blue-200 text-blue-800 text-[11px]">
              <svg className="h-4 w-4 shrink-0 text-blue-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <span>
                <strong>Host Device Diagnostics:</strong> Showing hardware and WebView info of the host device running the proxy APK.
              </span>
            </div>
          )}

          {/* 1. Android System WebView & LNA Status Card */}
          {(info.webViewPackage !== undefined || info.webViewVersion !== undefined || info.userAgent !== undefined) && (
            <div className="rounded-xl border border-gray-200 bg-gray-50/70 p-3.5 space-y-2.5">
              <div className="flex items-center justify-between gap-2 flex-wrap">
                <div className="flex items-center gap-2">
                  <span className="flex h-6 w-6 items-center justify-center rounded-md bg-blue-100 text-blue-700">
                    <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9" />
                    </svg>
                  </span>
                  <span className="text-xs font-semibold text-gray-800">
                    Android System WebView & LNA
                  </span>
                </div>

                {isModernChromium === true && (
                  <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-emerald-100 text-emerald-800">
                    <svg className="w-3 h-3 text-emerald-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M5 13l4 4L19 7" />
                    </svg>
                    Chromium {chromeMajor} (LNA Supported)
                  </span>
                )}

                {isModernChromium === false && (
                  <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-amber-100 text-amber-800">
                    <svg className="w-3 h-3 text-amber-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                    {chromeMajor !== null ? `Chromium v${chromeMajor}` : "Outdated / Non-Chromium WebView"}
                  </span>
                )}
              </div>

              <div className="grid grid-cols-2 gap-2 text-[11px] pt-1 border-t border-gray-200/80">
                {info.webViewPackage !== undefined ? (
                  <div>
                    <span className="text-gray-500">Provider Package:</span>
                    <div className="font-mono text-gray-800 truncate" title={info.webViewPackage}>
                      {info.webViewPackage}
                    </div>
                  </div>
                ) : (
                  <div>
                    <span className="text-gray-500">Provider Package:</span>
                    <div className="font-mono text-gray-400 italic">undefined</div>
                  </div>
                )}

                {info.webViewVersion !== undefined ? (
                  <div>
                    <span className="text-gray-500">WebView Engine Version:</span>
                    <div className="font-mono font-medium text-gray-800 truncate" title={info.webViewVersion}>
                      {info.webViewVersion}
                    </div>
                  </div>
                ) : (
                  <div>
                    <span className="text-gray-500">WebView Engine Version:</span>
                    <div className="font-mono text-gray-400 italic">undefined</div>
                  </div>
                )}
              </div>

              {isModernChromium === false && (
                <div className="bg-amber-50/90 border border-amber-200/80 rounded-lg p-2.5 text-[11px] text-amber-800 space-y-1">
                  <div className="font-semibold flex items-center gap-1.5">
                    <span>⚠️ Local Network Access Warning</span>
                  </div>
                  <p className="text-amber-700 leading-relaxed">
                    Odoo POS Local Network Access (LNA) requires modern Chromium (120+ or higher). An older WebView version causes the error <em>&quot;this is not a Chromium-based browser&quot;</em>. Update <strong>Android System WebView</strong> from Google Play Store or install the latest WebView APK on this device.
                  </p>
                </div>
              )}

              {info.userAgent !== undefined && (
                <div className="space-y-1">
                  <div className="flex items-center justify-between">
                    <span className="text-gray-500 text-[10px]">User-Agent string:</span>
                    <button
                      type="button"
                      onClick={() => copyUserAgent(info.userAgent || "")}
                      className="text-[10px] text-odoo hover:underline font-medium cursor-pointer"
                    >
                      Copy UA
                    </button>
                  </div>
                  <div className="p-1.5 rounded bg-white border border-gray-200 font-mono text-[10px] text-gray-600 break-all select-all">
                    {info.userAgent}
                  </div>
                </div>
              )}
            </div>
          )}

          {/* 2. Device Hardware & OS Details */}
          <div className="rounded-xl border border-gray-200 p-3.5 space-y-2.5">
            <div className="flex items-center gap-2">
              <span className="flex h-6 w-6 items-center justify-center rounded-md bg-purple-100 text-purple-700">
                <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 18h.01M8 21h8a2 2 0 002-2V5a2 2 0 00-2-2H8a2 2 0 00-2 2v14a2 2 0 002 2z" />
                </svg>
              </span>
              <span className="text-xs font-semibold text-gray-800">
                Device & Operating System
              </span>
            </div>

            <div className="grid grid-cols-2 sm:grid-cols-3 gap-2.5 text-[11px]">
              {info.model !== undefined && (
                <div className="bg-gray-50/80 rounded-lg p-2 border border-gray-100">
                  <span className="text-gray-400 block text-[10px]">Model</span>
                  <span className="font-semibold text-gray-800 truncate block">
                    {info.model}
                  </span>
                </div>
              )}

              {(info.manufacturer !== undefined || info.brand !== undefined) && (
                <div className="bg-gray-50/80 rounded-lg p-2 border border-gray-100">
                  <span className="text-gray-400 block text-[10px]">Manufacturer</span>
                  <span className="font-semibold text-gray-800 truncate block">
                    {info.manufacturer ?? info.brand}
                  </span>
                </div>
              )}

              {info.osVersion !== undefined && (
                <div className="bg-gray-50/80 rounded-lg p-2 border border-gray-100">
                  <span className="text-gray-400 block text-[10px]">OS Version</span>
                  <span className="font-semibold text-gray-800 truncate block">
                    {info.osVersion}
                  </span>
                </div>
              )}

              {info.apiLevel !== undefined && (
                <div className="bg-gray-50/80 rounded-lg p-2 border border-gray-100">
                  <span className="text-gray-400 block text-[10px]">API Level</span>
                  <span className="font-medium text-gray-800 block">
                    API {info.apiLevel}
                  </span>
                </div>
              )}

              {info.securityPatch !== undefined && (
                <div className="bg-gray-50/80 rounded-lg p-2 border border-gray-100">
                  <span className="text-gray-400 block text-[10px]">Security Patch</span>
                  <span className="font-medium text-gray-800 block">
                    {info.securityPatch}
                  </span>
                </div>
              )}

              {(info.board !== undefined || info.hardware !== undefined) && (
                <div className="bg-gray-50/80 rounded-lg p-2 border border-gray-100">
                  <span className="text-gray-400 block text-[10px]">Board / Hardware</span>
                  <span className="font-medium text-gray-800 truncate block">
                    {info.board ?? info.hardware}
                  </span>
                </div>
              )}
            </div>
          </div>

          {/* 3. Resources: RAM, Storage, CPU, Battery */}
          {(info.totalRamBytes !== undefined || info.totalStorageBytes !== undefined || info.cpuCores !== undefined || info.batteryLevel !== undefined) && (
            <div className="rounded-xl border border-gray-200 p-3.5 space-y-3">
              <div className="flex items-center gap-2">
                <span className="flex h-6 w-6 items-center justify-center rounded-md bg-emerald-100 text-emerald-700">
                  <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" />
                  </svg>
                </span>
                <span className="text-xs font-semibold text-gray-800">
                  Hardware Resources
                </span>
              </div>

              <div className="space-y-2.5">
                {/* RAM */}
                {info.totalRamBytes !== undefined && (
                  <div className="space-y-1">
                    <div className="flex justify-between items-center text-[11px]">
                      <span className="text-gray-600 font-medium">Memory (RAM)</span>
                      <span className="font-mono text-gray-800">
                        {ramUsedBytes !== undefined ? formatBytes(ramUsedBytes) : "undefined"} / {formatBytes(info.totalRamBytes)}
                        {ramPercent !== undefined ? ` (${ramPercent}%)` : ""}
                      </span>
                    </div>
                    {ramPercent !== undefined && (
                      <div className="w-full h-2 rounded-full bg-gray-100 overflow-hidden border border-gray-200/60">
                        <div
                          className={`h-full rounded-full transition-all duration-500 ${
                            ramPercent > 85 ? "bg-red-500" : ramPercent > 70 ? "bg-amber-500" : "bg-emerald-500"
                          }`}
                          style={{ width: `${ramPercent}%` }}
                        />
                      </div>
                    )}
                    {info.availableRamBytes !== undefined && (
                      <div className="flex justify-between text-[10px] text-gray-400">
                        <span>Available: {formatBytes(info.availableRamBytes)}</span>
                        {info.isLowRam === true && <span className="text-red-500 font-semibold">Low Memory Warning</span>}
                      </div>
                    )}
                  </div>
                )}

                {/* Internal Storage */}
                {info.totalStorageBytes !== undefined && (
                  <div className="space-y-1 pt-1 border-t border-gray-100">
                    <div className="flex justify-between items-center text-[11px]">
                      <span className="text-gray-600 font-medium">Internal Storage</span>
                      <span className="font-mono text-gray-800">
                        {storageUsedBytes !== undefined ? formatBytes(storageUsedBytes) : "undefined"} / {formatBytes(info.totalStorageBytes)}
                        {storagePercent !== undefined ? ` (${storagePercent}%)` : ""}
                      </span>
                    </div>
                    {storagePercent !== undefined && (
                      <div className="w-full h-2 rounded-full bg-gray-100 overflow-hidden border border-gray-200/60">
                        <div
                          className={`h-full rounded-full transition-all duration-500 ${
                            storagePercent > 90 ? "bg-red-500" : storagePercent > 75 ? "bg-amber-500" : "bg-blue-500"
                          }`}
                          style={{ width: `${storagePercent}%` }}
                        />
                      </div>
                    )}
                    {info.availableStorageBytes !== undefined && (
                      <div className="flex justify-between text-[10px] text-gray-400">
                        <span>Free: {formatBytes(info.availableStorageBytes)}</span>
                      </div>
                    )}
                  </div>
                )}

                {/* CPU & Battery Grid */}
                <div className="grid grid-cols-2 gap-2 pt-1 border-t border-gray-100 text-[11px]">
                  {info.cpuCores !== undefined && (
                    <div className="bg-gray-50/80 rounded-lg p-2 border border-gray-100">
                      <span className="text-gray-400 block text-[10px]">CPU</span>
                      <span className="font-semibold text-gray-800 block truncate">
                        {info.cpuCores} Cores{info.cpuArch ? ` (${info.cpuArch})` : ""}
                      </span>
                      {info.socModel !== undefined && (
                        <span className="text-[10px] text-gray-500 truncate block">
                          SoC: {info.socModel}
                        </span>
                      )}
                    </div>
                  )}

                  {info.batteryLevel !== undefined && (
                    <div className="bg-gray-50/80 rounded-lg p-2 border border-gray-100">
                      <span className="text-gray-400 block text-[10px]">Battery</span>
                      <div className="flex items-center gap-1.5 font-semibold text-gray-800">
                        <span>{info.batteryLevel}%</span>
                        {info.isCharging === true ? (
                          <span className="text-emerald-600 text-[10px] font-normal flex items-center">
                            ⚡ {info.pluggedSource ?? "Charging"}
                          </span>
                        ) : info.isCharging === false ? (
                          <span className="text-gray-500 text-[10px] font-normal">
                            Battery
                          </span>
                        ) : null}
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}

          {/* 4. Display & Connectivity */}
          {(info.screenWidth !== undefined || info.networkType !== undefined || info.localIp !== undefined) && (
            <div className="rounded-xl border border-gray-200 p-3.5 space-y-2.5">
              <div className="flex items-center gap-2">
                <span className="flex h-6 w-6 items-center justify-center rounded-md bg-indigo-100 text-indigo-700">
                  <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                  </svg>
                </span>
                <span className="text-xs font-semibold text-gray-800">
                  Display & Network
                </span>
              </div>

              <div className="grid grid-cols-2 gap-2 text-[11px]">
                {info.screenWidth !== undefined && info.screenHeight !== undefined && (
                  <div className="bg-gray-50/80 rounded-lg p-2 border border-gray-100">
                    <span className="text-gray-400 block text-[10px]">Screen Display</span>
                    <span className="font-semibold text-gray-800 block truncate">
                      {info.screenWidth} × {info.screenHeight}
                      {info.refreshRate !== undefined ? ` @ ${info.refreshRate}Hz` : ""}
                    </span>
                    {info.screenDensityDpi !== undefined && (
                      <span className="text-[10px] text-gray-500 block">
                        {info.screenDensityDpi} DPI{info.screenDensity !== undefined ? ` (${info.screenDensity.toFixed(1)}x)` : ""}
                      </span>
                    )}
                  </div>
                )}

                {(info.networkType !== undefined || info.localIp !== undefined) && (
                  <div className="bg-gray-50/80 rounded-lg p-2 border border-gray-100">
                    <span className="text-gray-400 block text-[10px]">Network Connection</span>
                    {info.networkType !== undefined && (
                      <span className="font-semibold text-gray-800 block truncate">
                        {info.networkType}
                      </span>
                    )}
                    {info.localIp !== undefined && (
                      <span className="text-[10px] text-gray-500 block font-mono truncate">
                        IP: {info.localIp}{info.port ? `:${info.port}` : ""}
                      </span>
                    )}
                  </div>
                )}
              </div>
            </div>
          )}

          {/* 5. App & System Uptime */}
          {(info.appVersion !== undefined || info.uptimeMs !== undefined) && (
            <div className="rounded-xl border border-gray-200 p-3 bg-gray-50/40 text-[11px] flex items-center justify-between flex-wrap gap-2">
              {info.appVersion !== undefined && (
                <div>
                  <span className="text-gray-500">App Version:</span>{" "}
                  <span className="font-mono font-medium text-gray-800">
                    v{info.appVersion}
                    {info.appVersionCode !== undefined ? ` (${info.appVersionCode})` : ""}
                  </span>
                </div>
              )}
              {info.uptimeMs !== undefined && formatUptime(info.uptimeMs) !== undefined && (
                <div>
                  <span className="text-gray-500">Device Uptime:</span>{" "}
                  <span className="font-mono font-medium text-gray-800">
                    {formatUptime(info.uptimeMs)}
                  </span>
                </div>
              )}
            </div>
          )}

        </div>
      ) : null}
    </Dialog>
  );
}
