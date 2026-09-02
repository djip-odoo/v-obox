package com.wails.app;

import android.app.Activity;
import android.app.ActivityManager;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.content.pm.PackageInfo;
import android.net.ConnectivityManager;
import android.net.NetworkInfo;
import android.os.BatteryManager;
import android.os.Build;
import android.os.Environment;
import android.os.StatFs;
import android.os.SystemClock;
import android.util.DisplayMetrics;
import android.util.Log;
import android.view.Display;
import java.io.File;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import android.webkit.JavascriptInterface;
import android.webkit.WebView;
import org.json.JSONArray;
import org.json.JSONObject;
import com.wails.app.BuildConfig;

/**
 * WailsJSBridge provides the JavaScript interface that allows the web frontend
 * to communicate with the Go backend. This is exposed to JavaScript as the
 * `window.wails` object.
 *
 * Similar to iOS's WKScriptMessageHandler but using Android's addJavascriptInterface.
 */
public class WailsJSBridge {
    private static final String TAG = "WailsJSBridge";
    private static final boolean DEBUG = BuildConfig.DEBUG;
    // Pooled threads avoid unbounded thread creation under high call volume.
    private static final ExecutorService executor = Executors.newCachedThreadPool();

    private final WailsBridge bridge;
    private final WebView webView;

    public WailsJSBridge(WailsBridge bridge, WebView webView) {
        this.bridge = bridge;
        this.webView = webView;
    }

    /**
     * Send a message to Go and return the response synchronously.
     * Called from JavaScript: wails.invoke(message)
     *
     * @param message The message to send (JSON string)
     * @return The response from Go (JSON string)
     */
    @JavascriptInterface
    public String invoke(String message) {
        if (DEBUG) Log.d(TAG, "Invoke called: " + message);
        return bridge.handleMessage(message);
    }

    /**
     * Send a message to Go asynchronously.
     * The response will be sent back via a callback.
     * Called from JavaScript: wails.invokeAsync(callbackId, message)
     *
     * @param callbackId The callback ID to use for the response
     * @param message The message to send (JSON string)
     */
    @JavascriptInterface
    public void invokeAsync(final String callbackId, final String payload) {
        if (DEBUG) Log.d(TAG, "InvokeAsync called: " + payload);

        // Handle off the JS thread so we don't block the WebView.
        executor.execute(() -> {
            try {
                String response = bridge.handleRuntimeCall(payload);
                sendCallback(callbackId, response, null);
            } catch (Exception e) {
                Log.e(TAG, "Error in async invoke", e);
                sendCallback(callbackId, null, e.getMessage());
            }
        });
    }

    /**
     * Log a message from JavaScript to Android's logcat
     * Called from JavaScript: wails.log(level, message)
     *
     * @param level The log level (debug, info, warn, error)
     * @param message The message to log
     */
    @JavascriptInterface
    public void log(String level, String message) {
        switch (level.toLowerCase()) {
            case "debug":
                Log.d(TAG + "/JS", message);
                break;
            case "info":
                Log.i(TAG + "/JS", message);
                break;
            case "warn":
                Log.w(TAG + "/JS", message);
                break;
            case "error":
                Log.e(TAG + "/JS", message);
                break;
            default:
                Log.v(TAG + "/JS", message);
                break;
        }
    }

    /**
     * Get the platform name
     * Called from JavaScript: wails.platform()
     *
     * @return "android"
     */
    @JavascriptInterface
    public String platform() {
        return "android";
    }

    /**
     * Check if we're running in debug mode
     * Called from JavaScript: wails.isDebug()
     *
     * @return true if debug build, false otherwise
     */
    @JavascriptInterface
    public boolean isDebug() {
        return BuildConfig.DEBUG;
    }

    /**
     * Set fullscreen / kiosk mode
     * Called from JavaScript: wails.setFullscreen(boolean)
     */
    @JavascriptInterface
    public void setFullscreen(boolean fullscreen) {
        if (bridge != null && bridge.getActivity() != null) {
            bridge.getActivity().runOnUiThread(() -> {
                if (bridge.getActivity() instanceof MainActivity) {
                    ((MainActivity) bridge.getActivity()).setFullscreenMode(fullscreen);
                }
            });
        }
    }

    /**
     * Quit application completely (prompts for new launcher if currently default launcher)
     * Called from JavaScript: wails.quitApp()
     */
    @JavascriptInterface
    public void quitApp() {
        if (bridge != null && bridge.getActivity() != null) {
            bridge.getActivity().runOnUiThread(() -> {
                try {
                    if (bridge.getActivity() instanceof MainActivity) {
                        ((MainActivity) bridge.getActivity()).quitAppWithLauncherPrompt();
                    } else {
                        bridge.stopForegroundService();
                        bridge.getActivity().finishAndRemoveTask();
                    }
                } catch (Exception e) {
                    Log.e(TAG, "Error quitting app", e);
                }
            });
        }
    }

    /**
     * Check if application is currently default home/launcher
     * Called from JavaScript: wails.isDefaultLauncher()
     */
    @JavascriptInterface
    public boolean isDefaultLauncher() {
        if (bridge != null && bridge.getActivity() instanceof MainActivity) {
            return ((MainActivity) bridge.getActivity()).isDefaultLauncher();
        }
        return false;
    }

    /**
     * Request default launcher / home app role or open settings
     * Called from JavaScript: wails.requestDefaultLauncher()
     */
    @JavascriptInterface
    public void requestDefaultLauncher() {
        if (bridge != null && bridge.getActivity() instanceof MainActivity) {
            ((MainActivity) bridge.getActivity()).requestDefaultLauncher();
        }
    }

    /**
     * Open Home Settings directly
     * Called from JavaScript: wails.openHomeSettings()
     */
    @JavascriptInterface
    public void openHomeSettings() {
        if (bridge != null && bridge.getActivity() instanceof MainActivity) {
            ((MainActivity) bridge.getActivity()).openHomeSettings();
        }
    }

    /**
     * Return comprehensive device hardware, OS, RAM, CPU, Storage, Display, and Android WebView information.
     * Called from JavaScript: wails.getDeviceInfo()
     *
     * @return JSON string containing device details
     */
    @JavascriptInterface
    public String getDeviceInfo() {
        JSONObject json = new JSONObject();
        try {
            Activity activity = bridge != null ? bridge.getActivity() : null;
            Context context = activity != null ? activity : (webView != null ? webView.getContext() : null);

            // Platform & Device Identity
            json.put("platform", "Android");
            if (Build.MODEL != null && !Build.MODEL.trim().isEmpty() && !Build.MODEL.equalsIgnoreCase("unknown")) {
                json.put("model", Build.MODEL);
            }
            if (Build.MANUFACTURER != null && !Build.MANUFACTURER.trim().isEmpty() && !Build.MANUFACTURER.equalsIgnoreCase("unknown")) {
                json.put("manufacturer", Build.MANUFACTURER);
            }
            if (Build.BRAND != null && !Build.BRAND.trim().isEmpty() && !Build.BRAND.equalsIgnoreCase("unknown")) {
                json.put("brand", Build.BRAND);
            }
            if (Build.DEVICE != null && !Build.DEVICE.trim().isEmpty() && !Build.DEVICE.equalsIgnoreCase("unknown")) {
                json.put("device", Build.DEVICE);
            }
            if (Build.PRODUCT != null && !Build.PRODUCT.trim().isEmpty() && !Build.PRODUCT.equalsIgnoreCase("unknown")) {
                json.put("product", Build.PRODUCT);
            }
            if (Build.BOARD != null && !Build.BOARD.trim().isEmpty() && !Build.BOARD.equalsIgnoreCase("unknown")) {
                json.put("board", Build.BOARD);
            }
            if (Build.HARDWARE != null && !Build.HARDWARE.trim().isEmpty() && !Build.HARDWARE.equalsIgnoreCase("unknown")) {
                json.put("hardware", Build.HARDWARE);
            }

            // OS & Security
            if (Build.VERSION.RELEASE != null && !Build.VERSION.RELEASE.isEmpty()) {
                json.put("osVersion", "Android " + Build.VERSION.RELEASE);
            }
            json.put("apiLevel", Build.VERSION.SDK_INT);
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M && Build.VERSION.SECURITY_PATCH != null && !Build.VERSION.SECURITY_PATCH.isEmpty()) {
                json.put("securityPatch", Build.VERSION.SECURITY_PATCH);
            }

            // WebView Info
            PackageInfo webViewPackage = null;
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                try {
                    webViewPackage = WebView.getCurrentWebViewPackage();
                } catch (Throwable ignored) {}
            }
            if (webViewPackage == null && context != null) {
                String[] candidatePackages = new String[]{
                        "com.google.android.webview",
                        "com.android.webview",
                        "com.google.android.chrome"
                };
                for (String pkg : candidatePackages) {
                    try {
                        webViewPackage = context.getPackageManager().getPackageInfo(pkg, 0);
                        if (webViewPackage != null) break;
                    } catch (Throwable ignored) {}
                }
            }
            if (webViewPackage != null) {
                if (webViewPackage.packageName != null && !webViewPackage.packageName.isEmpty()) {
                    json.put("webViewPackage", webViewPackage.packageName);
                }
                if (webViewPackage.versionName != null && !webViewPackage.versionName.isEmpty()) {
                    json.put("webViewVersion", webViewPackage.versionName);
                }
            }

            // User-Agent
            if (webView != null && webView.getSettings() != null) {
                String ua = webView.getSettings().getUserAgentString();
                if (ua != null && !ua.isEmpty()) {
                    json.put("userAgent", ua);
                }
            }

            // CPU & Architecture
            json.put("cpuCores", Runtime.getRuntime().availableProcessors());
            if (Build.SUPPORTED_ABIS != null && Build.SUPPORTED_ABIS.length > 0) {
                json.put("cpuArch", Build.SUPPORTED_ABIS[0]);
                JSONArray abis = new JSONArray();
                for (String abi : Build.SUPPORTED_ABIS) {
                    abis.put(abi);
                }
                json.put("supportedAbis", abis);
            } else if (Build.CPU_ABI != null && !Build.CPU_ABI.isEmpty()) {
                json.put("cpuArch", Build.CPU_ABI);
            }
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S && Build.SOC_MODEL != null && !Build.SOC_MODEL.isEmpty()) {
                try {
                    json.put("socModel", Build.SOC_MODEL);
                } catch (Throwable ignored) {}
            }

            // RAM / Memory
            if (context != null) {
                try {
                    ActivityManager am = (ActivityManager) context.getSystemService(Context.ACTIVITY_SERVICE);
                    if (am != null) {
                        ActivityManager.MemoryInfo memInfo = new ActivityManager.MemoryInfo();
                        am.getMemoryInfo(memInfo);
                        json.put("totalRamBytes", memInfo.totalMem);
                        json.put("availableRamBytes", memInfo.availMem);
                        json.put("usedRamBytes", memInfo.totalMem - memInfo.availMem);
                        json.put("isLowRam", memInfo.lowMemory);
                        json.put("ramThresholdBytes", memInfo.threshold);
                    }
                } catch (Throwable ignored) {}
            }

            // Internal Storage
            try {
                File dataDir = Environment.getDataDirectory();
                StatFs stat = new StatFs(dataDir.getPath());
                long blockSize = stat.getBlockSizeLong();
                long totalBlocks = stat.getBlockCountLong();
                long availBlocks = stat.getAvailableBlocksLong();
                long totalStorage = totalBlocks * blockSize;
                long availStorage = availBlocks * blockSize;
                json.put("totalStorageBytes", totalStorage);
                json.put("availableStorageBytes", availStorage);
                json.put("usedStorageBytes", totalStorage - availStorage);
            } catch (Throwable ignored) {}

            // Battery
            if (context != null) {
                try {
                    IntentFilter ifilter = new IntentFilter(Intent.ACTION_BATTERY_CHANGED);
                    Intent bIntent = context.registerReceiver(null, ifilter);
                    if (bIntent != null) {
                        int level = bIntent.getIntExtra(BatteryManager.EXTRA_LEVEL, -1);
                        int scale = bIntent.getIntExtra(BatteryManager.EXTRA_SCALE, -1);
                        int status = bIntent.getIntExtra(BatteryManager.EXTRA_STATUS, -1);
                        boolean isCharging = status == BatteryManager.BATTERY_STATUS_CHARGING ||
                                             status == BatteryManager.BATTERY_STATUS_FULL;
                        int plugged = bIntent.getIntExtra(BatteryManager.EXTRA_PLUGGED, -1);
                        String pluggedStr = null;
                        if (plugged == BatteryManager.BATTERY_PLUGGED_AC) pluggedStr = "AC Charger";
                        else if (plugged == BatteryManager.BATTERY_PLUGGED_USB) pluggedStr = "USB";
                        else if (plugged == BatteryManager.BATTERY_PLUGGED_WIRELESS) pluggedStr = "Wireless";

                        if (level >= 0 && scale > 0) {
                            int percent = Math.round((level / (float) scale) * 100);
                            json.put("batteryLevel", percent);
                            json.put("isCharging", isCharging);
                            if (pluggedStr != null) {
                                json.put("pluggedSource", pluggedStr);
                            }
                        }
                    }
                } catch (Throwable ignored) {}
            }

            // Display
            if (context != null) {
                try {
                    DisplayMetrics dm = context.getResources().getDisplayMetrics();
                    json.put("screenWidth", dm.widthPixels);
                    json.put("screenHeight", dm.heightPixels);
                    json.put("screenDensityDpi", dm.densityDpi);
                    json.put("screenDensity", dm.density);

                    if (activity != null) {
                        Display display = activity.getWindowManager().getDefaultDisplay();
                        json.put("refreshRate", Math.round(display.getRefreshRate()));
                    }
                } catch (Throwable ignored) {}
            }

            // Network
            if (context != null) {
                try {
                    ConnectivityManager cm = (ConnectivityManager) context.getSystemService(Context.CONNECTIVITY_SERVICE);
                    if (cm != null) {
                        NetworkInfo activeNetwork = cm.getActiveNetworkInfo();
                        if (activeNetwork != null && activeNetwork.isConnected() && activeNetwork.getTypeName() != null) {
                            json.put("networkType", activeNetwork.getTypeName());
                        }
                    }
                } catch (Throwable ignored) {}
            }

            // App Package Info
            if (context != null) {
                try {
                    PackageInfo pInfo = context.getPackageManager().getPackageInfo(context.getPackageName(), 0);
                    if (pInfo.packageName != null) json.put("packageName", pInfo.packageName);
                    if (pInfo.versionName != null) json.put("appVersion", pInfo.versionName);
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                        json.put("appVersionCode", pInfo.getLongVersionCode());
                    } else {
                        json.put("appVersionCode", pInfo.versionCode);
                    }
                } catch (Throwable ignored) {}
            }

            // Uptime
            json.put("uptimeMs", SystemClock.elapsedRealtime());

        } catch (Exception e) {
            Log.e(TAG, "Error collecting device info", e);
            try {
                json.put("error", e.getMessage());
            } catch (Exception ignored) {}
        }
        return json.toString();
    }

    /**
     * Send a callback response to JavaScript
     */
    private void sendCallback(String callbackId, String result, String error) {
        final String js;
        if (error != null) {
            js = String.format(
                    "window._wailsAndroidCallback && window._wailsAndroidCallback('%s', null, '%s');",
                    escapeJsString(callbackId),
                    escapeJsString(error)
            );
        } else {
            js = String.format(
                    "window._wailsAndroidCallback && window._wailsAndroidCallback('%s', '%s', null);",
                    escapeJsString(callbackId),
                    escapeJsString(result != null ? result : "")
            );
        }

        webView.post(() -> webView.evaluateJavascript(js, null));
    }

    private String escapeJsString(String str) {
        if (str == null) return "";
        return str.replace("\\", "\\\\")
                .replace("'", "\\'")
                .replace("\n", "\\n")
                .replace("\r", "\\r")
                // JS line terminators (U+2028/U+2029) must be escaped too; built via
                // (char) casts so the Java lexer does not reinterpret them as newlines.
                .replace(String.valueOf((char) 0x2028), "\\u2028")
                .replace(String.valueOf((char) 0x2029), "\\u2029");
    }
}
