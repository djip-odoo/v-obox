package com.wails.app;

import android.annotation.SuppressLint;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.content.res.Configuration;
import android.database.Cursor;
import android.net.ConnectivityManager;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.net.Uri;
import android.os.BatteryManager;
import android.os.Build;
import android.os.Bundle;
import android.os.PowerManager;
import android.content.pm.PackageManager;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.provider.MediaStore;
import android.provider.OpenableColumns;
import android.util.Base64;
import android.util.Log;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.webkit.WebChromeClient;
import android.webkit.ConsoleMessage;
import android.webkit.CookieManager;
import android.webkit.GeolocationPermissions;
import android.webkit.PermissionRequest;
import android.webkit.SslErrorHandler;
import android.webkit.ServiceWorkerController;
import android.webkit.ServiceWorkerWebSettings;
import android.webkit.RenderProcessGoneDetail;
import android.webkit.JsResult;
import android.webkit.JsPromptResult;
import android.net.http.SslError;

import android.app.ActivityManager;
import android.app.admin.DevicePolicyManager;
import android.content.ComponentName;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.View;
import android.view.WindowManager;
import androidx.annotation.Nullable;
import androidx.appcompat.app.AppCompatActivity;
import androidx.core.content.FileProvider;
import androidx.core.view.WindowCompat;
import androidx.core.view.WindowInsetsCompat;
import androidx.core.view.WindowInsetsControllerCompat;
import androidx.webkit.WebViewAssetLoader;

import org.json.JSONObject;

import java.io.File;
import java.io.FileOutputStream;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.util.ArrayList;
import java.util.List;

/**
 * MainActivity hosts the WebView and manages the Wails application lifecycle.
 * It uses WebViewAssetLoader to serve assets from the Go library without
 * requiring a network server.
 */
public class MainActivity extends AppCompatActivity {
    private static final String TAG = "WailsActivity";
    private static final boolean DEBUG = BuildConfig.DEBUG;
    private static final String WAILS_SCHEME = "https";
    private static final String WAILS_HOST = "wails.localhost";
    private static final int FILE_PICKER_REQUEST = 7001;

    private WebView webView;
    private WailsBridge bridge;
    // Battery: system-event receivers are registered only while the activity is
    // in the foreground (onStart) and torn down in onStop, so background battery/
    // network/screen broadcasts don't wake the app.
    private boolean systemReceiversRegistered = false;
    private WebViewAssetLoader assetLoader;

    // The Go-side dialog ID of the in-flight file picker (-1 when idle)
    private int pendingFilePickerCallbackID = -1;
    private static final int PHOTO_CAPTURE_REQUEST = 7002;
    private static final int VIDEO_CAPTURE_REQUEST = 7003;
    private static final int CAMERA_PERMISSION_REQUEST = 7010;
    private File pendingCaptureFile;
    private boolean pendingCaptureIsVideo;
    private volatile double currentWebappZoom = 1.0;

    private boolean isExternalWebapp(String url) {
        return url != null && !url.isEmpty() && !url.contains(WAILS_HOST) && !url.startsWith("http://127.0.0.1:4545");
    }

    // System-event sources (battery/power, screen lock, network). Registered in
    // onCreate, torn down in onDestroy. Each forwards a "system:*" event to JS
    // via the bridge.
    private BroadcastReceiver batteryReceiver;
    private BroadcastReceiver screenReceiver;
    private BroadcastReceiver powerSaveReceiver;
    private ConnectivityManager connectivityManager;
    private ConnectivityManager.NetworkCallback networkCallback;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        try {
            File filesDir = getFilesDir();
            if (filesDir != null) {
                if (!filesDir.exists()) filesDir.mkdirs();
                android.system.Os.setenv("FILES_DIR", filesDir.getAbsolutePath(), true);
                android.system.Os.setenv("HOME", filesDir.getAbsolutePath(), true);
            }
            File cacheDir = getCacheDir();
            if (cacheDir != null) {
                if (!cacheDir.exists()) cacheDir.mkdirs();
                android.system.Os.setenv("TMPDIR", cacheDir.getAbsolutePath(), true);
            }
        } catch (Exception ignored) {}

        setContentView(R.layout.activity_main);

        // Initialize the native Go library
        bridge = new WailsBridge(this);
        bridge.initialize();

        // Setup Device Owner / Lock Task policy on startup
        setupLockTaskPolicy();

        // Start background proxy foreground service
        bridge.startForegroundService("{\"title\":\"ePOS Proxy\",\"text\":\"ePOS Proxy service is active\"}");

        // Set up WebView
        setupWebView();

        // Start real-time remote action listener (open, close, reload)
        startWebviewActionPoller();

        // Load the application
        loadApplication();
    }

    @SuppressLint("SetJavaScriptEnabled")
    private void setupWebView() {
        webView = findViewById(R.id.webview);
        webView.setInitialScale(0);
        bridge.setWebView(webView);

        // Enable third-party cookies for seamless cross-origin and Odoo POS session syncing
        CookieManager cookieManager = CookieManager.getInstance();
        cookieManager.setAcceptCookie(true);
        cookieManager.setAcceptThirdPartyCookies(webView, true);

        // Configure WebView settings
        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setTextZoom(100);
        settings.setDomStorageEnabled(true);
        settings.setDatabaseEnabled(true);
        settings.setAllowFileAccess(true);
        settings.setAllowContentAccess(true);
        settings.setAllowFileAccessFromFileURLs(true);
        settings.setAllowUniversalAccessFromFileURLs(true);
        settings.setCacheMode(WebSettings.LOAD_DEFAULT);
        settings.setGeolocationEnabled(true);
        settings.setSupportZoom(true);
        settings.setBuiltInZoomControls(false);
        settings.setDisplayZoomControls(false);
        settings.setMediaPlaybackRequiresUserGesture(false);
        // Configure UserAgent to present as full Google Chrome Mobile (strip '; wv' and 'Version/4.0'
        // which mark it as an embedded webview and cause web apps like Odoo POS to disable LNA).
        String ua = settings.getUserAgentString();
        if (ua != null) {
            ua = ua.replace("; wv", "")
                   .replaceAll("Version/\\d+\\.\\d+\\s*", "")
                   .replaceAll("\\s+", " ")
                   .trim();
            settings.setUserAgentString(ua);
        }
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_ALWAYS_ALLOW);
        settings.setSafeBrowsingEnabled(false);
        settings.setJavaScriptCanOpenWindowsAutomatically(true);

        // Enable debugging in debug builds
        if (DEBUG) {
            WebView.setWebContentsDebuggingEnabled(true);
        }

        // Enable ServiceWorker caching and storage access on Android N+
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
            try {
                ServiceWorkerController swController = ServiceWorkerController.getInstance();
                ServiceWorkerWebSettings swSettings = swController.getServiceWorkerWebSettings();
                swSettings.setAllowContentAccess(true);
                swSettings.setAllowFileAccess(true);
                swSettings.setCacheMode(WebSettings.LOAD_DEFAULT);
            } catch (Exception ignored) {}
        }

        // Set up asset loader for serving local assets
        assetLoader = new WebViewAssetLoader.Builder()
                .setDomain(WAILS_HOST)
                .addPathHandler("/", new WailsPathHandler(bridge))
                .build();

        // Set up WebView client to intercept requests
        webView.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                String scheme = request.getUrl().getScheme();
                if ("http".equalsIgnoreCase(scheme) || "https".equalsIgnoreCase(scheme)) {
                    // Keep all HTTP/HTTPS navigation directly inside this WebView
                    return false;
                }
                try {
                    Intent intent = new Intent(Intent.ACTION_VIEW, request.getUrl());
                    view.getContext().startActivity(intent);
                    return true;
                } catch (Exception e) {
                    return true;
                }
            }

            @Override
            public void onReceivedSslError(WebView view, SslErrorHandler handler, SslError error) {
                // In POS / local network environments, proceed with self-signed or internal CA certs
                if (DEBUG) Log.w(TAG, "SSL error ignored for POS network: " + error.toString());
                handler.proceed();
            }

            @Override
            public boolean onRenderProcessGone(WebView view, RenderProcessGoneDetail detail) {
                Log.e(TAG, "WebView render process gone (didCrash=" + (detail != null && detail.didCrash()) + "). Recovering...");
                if (view != null) {
                    view.post(() -> {
                        try {
                            String currentUrl = view.getUrl();
                            if (currentUrl != null && !currentUrl.isEmpty()) {
                                view.loadUrl(currentUrl);
                            } else {
                                loadApplication();
                            }
                        } catch (Exception ignored) {}
                    });
                }
                return true; // Prevents the host Android app from terminating!
            }

            @Nullable
            @Override
            public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) {
                // Handle wails.localhost requests
                if (request.getUrl().getHost() != null &&
                        request.getUrl().getHost().equals(WAILS_HOST)) {

                    // For wails API calls (runtime, capabilities, etc.) pass the
                    // full URL including the query string, because
                    // WebViewAssetLoader.PathHandler strips query params
                    String path = request.getUrl().getPath();
                    if (path != null && path.startsWith("/wails/")) {
                        String fullPath = path;
                        String query = request.getUrl().getQuery();
                        if (query != null && !query.isEmpty()) {
                            fullPath = path + "?" + query;
                        }
                        if (DEBUG) Log.d(TAG, "Wails API call: " + fullPath);

                        byte[] data = bridge.serveAsset(fullPath, request.getMethod(), "{}");
                        if (data != null && data.length > 0) {
                            java.io.InputStream inputStream = new java.io.ByteArrayInputStream(data);
                            java.util.Map<String, String> headers = new java.util.HashMap<>();
                            headers.put("Access-Control-Allow-Origin", "*");
                            headers.put("Cache-Control", "no-cache");
                            headers.put("Content-Type", "application/json");

                            return new WebResourceResponse(
                                "application/json",
                                "UTF-8",
                                200,
                                "OK",
                                headers,
                                inputStream
                            );
                        }
                        // Return error response if data is null
                        return new WebResourceResponse(
                            "application/json",
                            "UTF-8",
                            500,
                            "Internal Error",
                            new java.util.HashMap<>(),
                            new java.io.ByteArrayInputStream("{}".getBytes())
                        );
                    }

                    // Stream captured photos/videos from the cache with HTTP Range
                    // support so <video> can seek/stream a clip of any length.
                    if (path != null && path.startsWith("/__capture__/")) {
                        return serveCaptureFile(path.substring("/__capture__/".length()), request);
                    }

                    // For regular assets, use the asset loader
                    return assetLoader.shouldInterceptRequest(request.getUrl());
                }

                return super.shouldInterceptRequest(view, request);
            }

            @Override
            public void onPageFinished(WebView view, String url) {
                super.onPageFinished(view, url);
                if (DEBUG) Log.d(TAG, "Page loaded: " + url);
                bridge.onPageFinished(url);
                // Now that JS listeners are mounted, push a snapshot of the
                // current battery / network / theme so the UI starts populated.
                emitSystemSnapshot();

                if (view != null && url != null) {
                    if (isExternalWebapp(url)) {
                        view.setInitialScale(0);
                        if (currentWebappZoom > 0 && Math.abs(currentWebappZoom - 1.0) > 0.01) {
                            view.evaluateJavascript(
                                "document.documentElement.style.zoom = '" + currentWebappZoom + "';", null);
                        } else {
                            view.evaluateJavascript(
                                "document.documentElement.style.zoom = '1.0';", null);
                        }
                    } else {
                        // Wails UI: always 100% standard mobile scale
                        view.setInitialScale(0);
                        view.evaluateJavascript(
                            "document.documentElement.style.zoom = '1.0';", null);
                    }
                }
            }
        });

        // Add JavaScript interface for Go communication
        webView.addJavascriptInterface(new WailsJSBridge(bridge, webView), "wails");

        // Forward console logs to logcat and handle webapp permissions / dialogs
        webView.setWebChromeClient(new WebChromeClient() {
            @Override
            public boolean onConsoleMessage(ConsoleMessage consoleMessage) {
                Log.d("Wails/JS", consoleMessage.message() + " -- From line "
                        + consoleMessage.lineNumber() + " of "
                        + consoleMessage.sourceId());
                return true;
            }

            @Override
            public boolean onJsBeforeUnload(WebView view, String url, String message, JsResult result) {
                // Automatically confirm reload/navigation so location.reload() never blocks or hangs
                result.confirm();
                return true;
            }

            @Override
            public boolean onJsAlert(WebView view, String url, String message, JsResult result) {
                try {
                    new androidx.appcompat.app.AlertDialog.Builder(MainActivity.this)
                        .setMessage(message)
                        .setPositiveButton(android.R.string.ok, (dialog, which) -> result.confirm())
                        .setOnCancelListener(dialog -> result.cancel())
                        .show();
                } catch (Exception e) {
                    result.confirm();
                }
                return true;
            }

            @Override
            public boolean onJsConfirm(WebView view, String url, String message, JsResult result) {
                try {
                    new androidx.appcompat.app.AlertDialog.Builder(MainActivity.this)
                        .setMessage(message)
                        .setPositiveButton(android.R.string.ok, (dialog, which) -> result.confirm())
                        .setNegativeButton(android.R.string.cancel, (dialog, which) -> result.cancel())
                        .setOnCancelListener(dialog -> result.cancel())
                        .show();
                } catch (Exception e) {
                    result.confirm();
                }
                return true;
            }

            @Override
            public boolean onJsPrompt(WebView view, String url, String message, String defaultValue, JsPromptResult result) {
                try {
                    final android.widget.EditText input = new android.widget.EditText(MainActivity.this);
                    input.setText(defaultValue);
                    new androidx.appcompat.app.AlertDialog.Builder(MainActivity.this)
                        .setMessage(message)
                        .setView(input)
                        .setPositiveButton(android.R.string.ok, (dialog, which) -> result.confirm(input.getText().toString()))
                        .setNegativeButton(android.R.string.cancel, (dialog, which) -> result.cancel())
                        .setOnCancelListener(dialog -> result.cancel())
                        .show();
                } catch (Exception e) {
                    result.confirm(defaultValue != null ? defaultValue : "");
                }
                return true;
            }

            @Override
            public void onPermissionRequest(final PermissionRequest request) {
                runOnUiThread(() -> request.grant(request.getResources()));
            }

            @Override
            public void onGeolocationPermissionsShowPrompt(String origin, GeolocationPermissions.Callback callback) {
                callback.invoke(origin, true, false);
            }
        });
    }

    private void loadApplication() {
        String url = WAILS_SCHEME + "://" + WAILS_HOST + "/";
        if (DEBUG) Log.d(TAG, "Loading URL: " + url);
        if (webView != null) {
            webView.setInitialScale(0);
            webView.evaluateJavascript("document.documentElement.style.zoom = '1.0';", null);
            webView.loadUrl(url);
        }
    }

    /**
     * Launch the system camera to capture a photo (video=false) or a video
     * (video=true). The capture is written to a FileProvider URI in the cache and
     * the result is delivered to JS as a "common:capture" event.
     */
    public void launchCameraCapture(boolean video) {
        if (checkSelfPermission("android.permission.CAMERA") != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{"android.permission.CAMERA"}, CAMERA_PERMISSION_REQUEST);
            bridge.emitEvent("common:capture",
                    "{\"error\":\"camera permission requested \u2014 tap again once granted\"}");
            return;
        }
        try {
            File dir = new File(getCacheDir(), "captures");
            if (!dir.exists()) dir.mkdirs();
            pendingCaptureFile = new File(dir, "capture_" + System.currentTimeMillis() + (video ? ".mp4" : ".jpg"));
            pendingCaptureIsVideo = video;
            Uri uri = FileProvider.getUriForFile(this, getPackageName() + ".fileprovider", pendingCaptureFile);
            Intent intent = new Intent(video ? MediaStore.ACTION_VIDEO_CAPTURE : MediaStore.ACTION_IMAGE_CAPTURE);
            intent.putExtra(MediaStore.EXTRA_OUTPUT, uri);
            intent.addFlags(Intent.FLAG_GRANT_WRITE_URI_PERMISSION);
            // Don't pre-check with resolveActivity(): Android 11+ package visibility
            // hides other apps' intents unless declared in <queries>, so it can
            // return null even when a camera app exists. Just launch and handle a miss.
            startActivityForResult(intent, video ? VIDEO_CAPTURE_REQUEST : PHOTO_CAPTURE_REQUEST);
        } catch (android.content.ActivityNotFoundException e) {
            bridge.emitEvent("common:capture", "{\"error\":\"no camera app available\"}");
        } catch (Exception e) {
            Log.e(TAG, "launchCameraCapture failed", e);
            bridge.emitEvent("common:capture", "{\"error\":\"capture failed\"}");
        }
    }

    private void handleCaptureResult(int resultCode, @Nullable Intent data) {
        File file = pendingCaptureFile;
        final boolean video = pendingCaptureIsVideo;
        pendingCaptureFile = null;
        if (resultCode != RESULT_OK) {
            bridge.emitEvent("common:capture", "{\"cancelled\":true}");
            return;
        }
        // Some camera apps (commonly for video) ignore EXTRA_OUTPUT and instead
        // return a content URI in the result data; copy that into our cache.
        if ((file == null || !file.exists() || file.length() == 0)
                && data != null && data.getData() != null) {
            String copied = copyUriToCache(data.getData());
            if (copied != null) file = new File(copied);
        }
        final File f = file;
        if (f == null || !f.exists() || f.length() == 0) {
            bridge.emitEvent("common:capture", "{\"cancelled\":true}");
            return;
        }
        new Thread(() -> {
            try {
                JSONObject o = new JSONObject();
                o.put("type", video ? "video" : "photo");
                o.put("path", f.getAbsolutePath());
                o.put("size", f.length());
                if (!video) {
                    String thumb = makePhotoThumbnail(f);
                    if (thumb != null) o.put("thumb", thumb);
                }
                // Stream URL works for both: <video>/<img> load it from the cache
                // via shouldInterceptRequest (Range-enabled), no size limit.
                o.put("streamUrl", captureStreamUrl(f));
                bridge.emitEvent("common:capture", o.toString());
            } catch (Exception e) {
                Log.e(TAG, "handleCaptureResult failed", e);
                bridge.emitEvent("common:capture", "{\"error\":\"result processing failed\"}");
            }
        }).start();
    }

    /** Downscale a captured photo into a base64 JPEG data URL for display in the webview. */
    @Nullable
    private String makePhotoThumbnail(File file) {
        try {
            BitmapFactory.Options bounds = new BitmapFactory.Options();
            bounds.inJustDecodeBounds = true;
            BitmapFactory.decodeFile(file.getAbsolutePath(), bounds);
            int sample = 1;
            while (Math.max(bounds.outWidth, bounds.outHeight) / sample > 640) sample *= 2;
            BitmapFactory.Options opts = new BitmapFactory.Options();
            opts.inSampleSize = sample;
            Bitmap bmp = BitmapFactory.decodeFile(file.getAbsolutePath(), opts);
            if (bmp == null) return null;
            ByteArrayOutputStream baos = new ByteArrayOutputStream();
            bmp.compress(Bitmap.CompressFormat.JPEG, 70, baos);
            bmp.recycle();
            return "data:image/jpeg;base64," + Base64.encodeToString(baos.toByteArray(), Base64.NO_WRAP);
        } catch (Exception e) {
            return null;
        }
    }

    /**
     * Build a same-origin URL the webview can stream a capture from. Served by
     * serveCaptureFile (via shouldInterceptRequest); the path is relative to the
     * cache dir so both camera files (captures/) and copied content URIs
     * (wails-picker/) resolve.
     */
    private String captureStreamUrl(File file) {
        String base = getCacheDir().getAbsolutePath() + File.separator;
        String abs = file.getAbsolutePath();
        String rel = abs.startsWith(base) ? abs.substring(base.length()) : file.getName();
        return "/__capture__/" + Uri.encode(rel, "/");
    }

    /**
     * Serve a captured file (under the app cache) to the webview with HTTP Range
     * support, so &lt;video&gt; can stream and seek a clip of any length without
     * inlining it as a data URL.
     */
    private WebResourceResponse serveCaptureFile(String relPath, WebResourceRequest request) {
        try {
            File cache = getCacheDir();
            File file = new File(cache, Uri.decode(relPath));
            // Path-traversal guard: only ever serve files under the cache dir.
            if (!file.getCanonicalPath().startsWith(cache.getCanonicalPath() + File.separator)
                    || !file.exists() || !file.isFile()) {
                return new WebResourceResponse("text/plain", "UTF-8", 404, "Not Found",
                        new java.util.HashMap<>(), new java.io.ByteArrayInputStream(new byte[0]));
            }
            String name = file.getName().toLowerCase();
            String mime = name.endsWith(".mp4") ? "video/mp4"
                    : name.endsWith(".mov") ? "video/quicktime"
                    : name.endsWith(".jpg") || name.endsWith(".jpeg") ? "image/jpeg"
                    : name.endsWith(".png") ? "image/png" : "application/octet-stream";
            long length = file.length();
            java.util.Map<String, String> reqHeaders = request.getRequestHeaders();
            String range = reqHeaders != null ? reqHeaders.get("Range") : null;
            if (range == null && reqHeaders != null) range = reqHeaders.get("range");

            java.util.Map<String, String> headers = new java.util.HashMap<>();
            headers.put("Accept-Ranges", "bytes");
            headers.put("Cache-Control", "no-store");

            if (range != null && range.startsWith("bytes=")) {
                long start = 0, end = length - 1;
                String spec = range.substring(6).trim();
                int dash = spec.indexOf('-');
                if (dash >= 0) {
                    try {
                        if (dash > 0) start = Long.parseLong(spec.substring(0, dash).trim());
                        String e = spec.substring(dash + 1).trim();
                        if (!e.isEmpty()) end = Long.parseLong(e);
                    } catch (NumberFormatException ignored) { }
                }
                if (start < 0) start = 0;
                if (end >= length) end = length - 1;
                if (start > end) { start = 0; end = length - 1; }
                long count = end - start + 1;
                java.io.InputStream in = new java.io.FileInputStream(file);
                long toSkip = start;
                while (toSkip > 0) {
                    long s = in.skip(toSkip);
                    if (s <= 0) break;
                    toSkip -= s;
                }
                headers.put("Content-Range", "bytes " + start + "-" + end + "/" + length);
                headers.put("Content-Length", String.valueOf(count));
                return new WebResourceResponse(mime, null, 206, "Partial Content",
                        headers, new LimitedInputStream(in, count));
            }
            headers.put("Content-Length", String.valueOf(length));
            return new WebResourceResponse(mime, null, 200, "OK", headers,
                    new java.io.FileInputStream(file));
        } catch (Exception e) {
            Log.e(TAG, "serveCaptureFile failed", e);
            return new WebResourceResponse("text/plain", "UTF-8", 500, "Error",
                    new java.util.HashMap<>(), new java.io.ByteArrayInputStream(new byte[0]));
        }
    }

    /** Wraps a stream to yield at most a fixed number of bytes (for Range responses). */
    private static final class LimitedInputStream extends java.io.FilterInputStream {
        private long remaining;
        LimitedInputStream(java.io.InputStream in, long limit) {
            super(in);
            this.remaining = limit;
        }
        @Override public int read() throws java.io.IOException {
            if (remaining <= 0) return -1;
            int b = super.read();
            if (b >= 0) remaining--;
            return b;
        }
        @Override public int read(byte[] b, int off, int len) throws java.io.IOException {
            if (remaining <= 0) return -1;
            int n = super.read(b, off, (int) Math.min(len, remaining));
            if (n > 0) remaining -= n;
            return n;
        }
    }

    /**
     * Launch the system document picker. Results are copied into the app's
     * cache directory so Go receives real filesystem paths. Called by
     * WailsBridge on the main thread.
     */
    public void launchFilePicker(int callbackID, boolean multiple) {
        synchronized (this) {
            if (pendingFilePickerCallbackID != -1) {
                // Only one picker can be in flight
                bridge.filePickerDone(callbackID);
                return;
            }
            pendingFilePickerCallbackID = callbackID;
        }

        Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType("*/*");
        intent.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, multiple);
        try {
            startActivityForResult(intent, FILE_PICKER_REQUEST);
        } catch (Exception e) {
            Log.e(TAG, "Failed to launch file picker", e);
            pendingFilePickerCallbackID = -1;
            bridge.filePickerDone(callbackID);
        }
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, @Nullable Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode == PHOTO_CAPTURE_REQUEST || requestCode == VIDEO_CAPTURE_REQUEST) {
            handleCaptureResult(resultCode, data);
            return;
        }
        if (requestCode != FILE_PICKER_REQUEST) {
            return;
        }
        final int callbackID = pendingFilePickerCallbackID;
        pendingFilePickerCallbackID = -1;
        if (callbackID == -1) {
            return;
        }

        final List<Uri> uris = new ArrayList<>();
        if (resultCode == RESULT_OK && data != null) {
            if (data.getClipData() != null) {
                for (int i = 0; i < data.getClipData().getItemCount(); i++) {
                    uris.add(data.getClipData().getItemAt(i).getUri());
                }
            } else if (data.getData() != null) {
                uris.add(data.getData());
            }
        }

        // Copy the documents off the main thread, then notify Go
        new Thread(() -> {
            for (Uri uri : uris) {
                String path = copyUriToCache(uri);
                if (path != null) {
                    bridge.filePickerResult(callbackID, path);
                }
            }
            bridge.filePickerDone(callbackID);
        }).start();
    }

    /**
     * Copy a content URI into the app cache and return its filesystem path.
     */
    @Nullable
    private String copyUriToCache(Uri uri) {
        String name = "document";
        try (Cursor cursor = getContentResolver().query(uri, null, null, null, null)) {
            if (cursor != null && cursor.moveToFirst()) {
                int idx = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME);
                if (idx >= 0 && cursor.getString(idx) != null) {
                    name = new File(cursor.getString(idx)).getName();
                }
            }
        } catch (Exception ignored) {
        }

        try {
            File dir = new File(getCacheDir(), "wails-picker/" + System.nanoTime());
            if (!dir.mkdirs()) {
                return null;
            }
            File out = new File(dir, name);
            try (InputStream in = getContentResolver().openInputStream(uri);
                 OutputStream os = new FileOutputStream(out)) {
                if (in == null) {
                    return null;
                }
                byte[] buf = new byte[64 * 1024];
                int n;
                while ((n = in.read(buf)) > 0) {
                    os.write(buf, 0, n);
                }
            }
            return out.getAbsolutePath();
        } catch (Exception e) {
            Log.e(TAG, "Failed to copy picked document", e);
            return null;
        }
    }

    /**
     * Apply display zoom strictly to the external Web App without affecting the Wails UI,
     * and without using setInitialScale which forces desktop viewport mode on mobile.
     */
    public void applyWebViewZoom(double zoom) {
        if (zoom <= 0) zoom = 1.0;
        this.currentWebappZoom = zoom;
        runOnUiThread(() -> {
            if (webView != null) {
                String currentUrl = webView.getUrl();
                if (isExternalWebapp(currentUrl)) {
                    webView.setInitialScale(0);
                    if (Math.abs(currentWebappZoom - 1.0) > 0.01) {
                        webView.evaluateJavascript(
                            "document.documentElement.style.zoom = '" + currentWebappZoom + "';", null);
                    } else {
                        webView.evaluateJavascript(
                            "document.documentElement.style.zoom = '1.0';", null);
                    }
                } else {
                    // Wails UI must always remain at standard 1.0 scale
                    webView.setInitialScale(0);
                    webView.evaluateJavascript(
                        "document.documentElement.style.zoom = '1.0';", null);
                }
            }
        });
    }

    /**
     * Execute JavaScript in the WebView from the Go side
     */
    public void executeJavaScript(final String js) {
        runOnUiThread(() -> {
            if (webView != null) {
                webView.evaluateJavascript(js, null);
            }
        });
    }

    // ---- System events ---------------------------------------------------
    // Battery/power, screen lock and network connectivity are surfaced to JS as
    // "system:*" events. The OS broadcasts used here (ACTION_BATTERY_CHANGED,
    // SCREEN_OFF, USER_PRESENT, POWER_SAVE_MODE_CHANGED) are protected system
    // broadcasts, so dynamic registration needs no RECEIVER_* export flag.

    private void registerSystemEventReceivers() {
        // Battery + charging state (sticky broadcast: the current value is
        // delivered to the receiver immediately on registration).
        batteryReceiver = new BroadcastReceiver() {
            @Override public void onReceive(Context context, Intent intent) {
                emitBattery(intent);
            }
        };
        registerReceiver(batteryReceiver, new IntentFilter(Intent.ACTION_BATTERY_CHANGED));

        // Low-power (battery saver) mode toggles → re-emit battery with the flag.
        powerSaveReceiver = new BroadcastReceiver() {
            @Override public void onReceive(Context context, Intent intent) {
                emitBattery(registerSticky(Intent.ACTION_BATTERY_CHANGED));
            }
        };
        registerReceiver(powerSaveReceiver,
                new IntentFilter(PowerManager.ACTION_POWER_SAVE_MODE_CHANGED));

        // Screen lock / unlock. SCREEN_OFF ≈ locked; USER_PRESENT = unlocked.
        screenReceiver = new BroadcastReceiver() {
            @Override public void onReceive(Context context, Intent intent) {
                String action = intent.getAction();
                if (Intent.ACTION_SCREEN_OFF.equals(action)) {
                    emitLock(true);
                } else if (Intent.ACTION_USER_PRESENT.equals(action)) {
                    emitLock(false);
                }
            }
        };
        IntentFilter screenFilter = new IntentFilter();
        screenFilter.addAction(Intent.ACTION_SCREEN_OFF);
        screenFilter.addAction(Intent.ACTION_USER_PRESENT);
        registerReceiver(screenReceiver, screenFilter);

        // Network connectivity / transport type / cellular signal strength.
        connectivityManager = (ConnectivityManager) getSystemService(Context.CONNECTIVITY_SERVICE);
        if (connectivityManager != null) {
            networkCallback = new ConnectivityManager.NetworkCallback() {
                @Override public void onAvailable(Network network) { emitNetwork(network); }
                @Override public void onLost(Network network) { emitNetworkDisconnected(); }
                @Override public void onCapabilitiesChanged(Network network, NetworkCapabilities caps) {
                    emitNetwork(network);
                }
            };
            try {
                connectivityManager.registerDefaultNetworkCallback(networkCallback);
            } catch (Exception e) {
                Log.e(TAG, "registerDefaultNetworkCallback failed", e);
            }
        }
    }

    private void unregisterSystemEventReceivers() {
        safeUnregister(batteryReceiver);
        batteryReceiver = null;
        safeUnregister(powerSaveReceiver);
        powerSaveReceiver = null;
        safeUnregister(screenReceiver);
        screenReceiver = null;
        if (connectivityManager != null && networkCallback != null) {
            try {
                connectivityManager.unregisterNetworkCallback(networkCallback);
            } catch (Exception ignored) {
            }
            networkCallback = null;
        }
    }

    private void safeUnregister(BroadcastReceiver r) {
        if (r != null) {
            try {
                unregisterReceiver(r);
            } catch (Exception ignored) {
            }
        }
    }

    /** Read the current sticky value for an action without a standing receiver. */
    @Nullable
    private Intent registerSticky(String action) {
        return registerReceiver(null, new IntentFilter(action));
    }

    /** Push current battery / network / theme so a freshly-loaded UI is populated. */
    private void emitSystemSnapshot() {
        emitBattery(registerSticky(Intent.ACTION_BATTERY_CHANGED));
        if (connectivityManager != null) {
            Network active = connectivityManager.getActiveNetwork();
            if (active != null) {
                emitNetwork(active);
            } else {
                emitNetworkDisconnected();
            }
        }
        emitTheme();
    }

    private void emitBattery(@Nullable Intent batteryStatus) {
        try {
            float level = -1f;
            String state = "unknown";
            if (batteryStatus != null) {
                int lvl = batteryStatus.getIntExtra(BatteryManager.EXTRA_LEVEL, -1);
                int scale = batteryStatus.getIntExtra(BatteryManager.EXTRA_SCALE, -1);
                if (lvl >= 0 && scale > 0) {
                    level = lvl / (float) scale;
                }
                switch (batteryStatus.getIntExtra(BatteryManager.EXTRA_STATUS, -1)) {
                    case BatteryManager.BATTERY_STATUS_CHARGING: state = "charging"; break;
                    case BatteryManager.BATTERY_STATUS_FULL: state = "full"; break;
                    case BatteryManager.BATTERY_STATUS_DISCHARGING:
                    case BatteryManager.BATTERY_STATUS_NOT_CHARGING: state = "unplugged"; break;
                    default: state = "unknown"; break;
                }
            }
            boolean lowPower = false;
            PowerManager pm = (PowerManager) getSystemService(Context.POWER_SERVICE);
            if (pm != null) {
                lowPower = pm.isPowerSaveMode();
            }
            JSONObject o = new JSONObject();
            o.put("level", (double) level);
            o.put("state", state);
            o.put("lowPowerMode", lowPower);
            if (bridge != null) bridge.emitSystemEvent("android:BatteryChanged", o.toString());
        } catch (Exception e) {
            Log.e(TAG, "emitBattery failed", e);
        }
    }

    private void emitNetwork(@Nullable Network network) {
        try {
            boolean connected = false;
            String type = "none";
            boolean metered = false;
            Integer signal = null;
            if (connectivityManager != null && network != null) {
                NetworkCapabilities caps = connectivityManager.getNetworkCapabilities(network);
                if (caps != null) {
                    connected = caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET);
                    if (caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)) {
                        type = "wifi";
                    } else if (caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)) {
                        type = "cellular";
                    } else if (caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET)) {
                        type = "wired";
                    } else {
                        type = "other";
                    }
                    metered = !caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED);
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                        int s = caps.getSignalStrength();
                        if (s != Integer.MIN_VALUE) {
                            signal = s; // dBm; closer to 0 is a stronger signal
                        }
                    }
                }
            }
            JSONObject o = new JSONObject();
            o.put("connected", connected);
            o.put("type", type);
            o.put("metered", metered);
            if (signal != null) {
                o.put("signal", (int) signal);
            }
            if (bridge != null) bridge.emitSystemEvent("android:NetworkChanged", o.toString());
        } catch (Exception e) {
            Log.e(TAG, "emitNetwork failed", e);
        }
    }

    private void emitNetworkDisconnected() {
        try {
            JSONObject o = new JSONObject();
            o.put("connected", false);
            o.put("type", "none");
            o.put("metered", false);
            if (bridge != null) bridge.emitSystemEvent("android:NetworkChanged", o.toString());
        } catch (Exception ignored) {
        }
    }

    private void emitLock(boolean locked) {
        // Lock/unlock are signals (no payload); name carries the state.
        if (bridge != null) {
            bridge.emitSystemEvent(locked ? "android:ScreenLocked" : "android:ScreenUnlocked", "{}");
        }
    }

    private void emitTheme() {
        try {
            int mode = getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK;
            JSONObject o = new JSONObject();
            // "isDarkMode" matches the context key the desktop platforms use.
            o.put("isDarkMode", mode == Configuration.UI_MODE_NIGHT_YES);
            if (bridge != null) bridge.emitSystemEvent("android:ThemeChanged", o.toString());
        } catch (Exception ignored) {
        }
    }

    @Override
    public void onConfigurationChanged(Configuration newConfig) {
        super.onConfigurationChanged(newConfig);
        // Fires for light/dark switches because the manifest lists uiMode in
        // android:configChanges (otherwise the activity would be recreated).
        emitTheme();
    }

    @Override
    protected void onStart() {
        super.onStart();
        // Battery: only monitor system events while the app is visible.
        if (!systemReceiversRegistered) {
            registerSystemEventReceivers();
            systemReceiversRegistered = true;
        }
        if (bridge != null) {
            bridge.onStart();
        }
    }

    @Override
    protected void onResume() {
        super.onResume();
        if (isKioskFullscreen) {
            setFullscreenMode(true);
        }
        if (bridge != null) {
            bridge.onResume();
        }
    }

    @Override
    protected void onPause() {
        super.onPause();
        if (bridge != null) {
            bridge.onPause();
        }
    }

    @Override
    protected void onStop() {
        super.onStop();
        if (systemReceiversRegistered) {
            unregisterSystemEventReceivers();
            systemReceiversRegistered = false;
        }
        if (bridge != null) {
            bridge.onStop();
        }
    }

    @Override
    public void onLowMemory() {
        super.onLowMemory();
        if (bridge != null) {
            bridge.onLowMemory();
        }
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        unregisterSystemEventReceivers();
        if (bridge != null) {
            bridge.shutdown();
        }
        if (webView != null) {
            webView.destroy();
        }
    }

    @Override
    public void onWindowFocusChanged(boolean hasFocus) {
        super.onWindowFocusChanged(hasFocus);
        if (isKioskFullscreen) {
            applyImmersiveMode(true);
            if (!hasFocus) {
                collapseStatusBar();
            }
        }
    }

    private boolean isKioskFullscreen = false;

    public void setFullscreenMode(boolean fullscreen) {
        this.isKioskFullscreen = fullscreen;
        runOnUiThread(() -> {
            try {
                if (fullscreen) {
                    getWindow().addFlags(WindowManager.LayoutParams.FLAG_FULLSCREEN);
                    getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                        getWindow().getAttributes().layoutInDisplayCutoutMode =
                            WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES;
                    }
                    startKioskLockTask();
                } else {
                    getWindow().clearFlags(WindowManager.LayoutParams.FLAG_FULLSCREEN);
                    getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
                    stopKioskLockTask();
                }

                applyImmersiveMode(fullscreen);
            } catch (Exception e) {
                Log.e(TAG, "Failed to set fullscreen mode", e);
            }
        });
    }

    public boolean isDeviceOwner() {
        try {
            DevicePolicyManager dpm = (DevicePolicyManager) getSystemService(Context.DEVICE_POLICY_SERVICE);
            return dpm != null && dpm.isDeviceOwnerApp(getPackageName());
        } catch (Exception e) {
            Log.w(TAG, "Error checking device owner: " + e.getMessage());
            return false;
        }
    }

    public boolean isLockTaskPermitted() {
        try {
            DevicePolicyManager dpm = (DevicePolicyManager) getSystemService(Context.DEVICE_POLICY_SERVICE);
            return dpm != null && dpm.isLockTaskPermitted(getPackageName());
        } catch (Exception e) {
            Log.w(TAG, "Error checking lock task permitted: " + e.getMessage());
            return false;
        }
    }

    public int getLockTaskModeState() {
        try {
            ActivityManager am = (ActivityManager) getSystemService(Context.ACTIVITY_SERVICE);
            if (am != null) {
                return am.getLockTaskModeState();
            }
        } catch (Exception e) {
            Log.w(TAG, "Error checking lock task state: " + e.getMessage());
        }
        return ActivityManager.LOCK_TASK_MODE_NONE;
    }

    public void setupLockTaskPolicy() {
        try {
            DevicePolicyManager dpm = (DevicePolicyManager) getSystemService(Context.DEVICE_POLICY_SERVICE);
            if (dpm == null) {
                Log.w(TAG, "DevicePolicyManager is null");
                return;
            }

            ComponentName admin = KioskDeviceAdminReceiver.getComponentName(this);
            boolean isOwner = dpm.isDeviceOwnerApp(getPackageName());
            Log.i(TAG, "Kiosk Policy Check: isDeviceOwner=" + isOwner);

            if (isOwner) {
                Log.i(TAG, "Device Owner active. Setting Lock Task packages & restrictive features.");
                // Whitelist this app package for Lock Task mode
                dpm.setLockTaskPackages(admin, new String[]{ getPackageName() });

                // Restrict all system features (Home, Recents, Notifications, System Info, Keyguard) in Lock Task
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                    dpm.setLockTaskFeatures(admin, DevicePolicyManager.LOCK_TASK_FEATURE_NONE);
                }

                // Disable keyguard
                dpm.setKeyguardDisabled(admin, true);

                Log.i(TAG, "LockTask configured. isLockTaskPermitted=" + dpm.isLockTaskPermitted(getPackageName()));
            } else {
                Log.w(TAG, "Application is NOT provisioned as Device Owner. Lock Task kiosk mode will not be enforced at system level.");
            }
        } catch (Exception e) {
            Log.e(TAG, "Error setting up lock task policy", e);
        }
    }

    public void startKioskLockTask() {
        try {
            setupLockTaskPolicy();

            int state = getLockTaskModeState();
            if (state == ActivityManager.LOCK_TASK_MODE_LOCKED) {
                Log.i(TAG, "LockTask already active in LOCKED state (mLockTaskModeState=LOCKED).");
                return;
            }

            if (isLockTaskPermitted()) {
                Log.i(TAG, "Calling startLockTask() -> expecting mLockTaskModeState=LOCKED");
                startLockTask();
            } else if (isDeviceOwner()) {
                setupLockTaskPolicy();
                if (isLockTaskPermitted()) {
                    startLockTask();
                }
            } else {
                Log.i(TAG, "Device is NOT Device Owner. Skipping startLockTask() to avoid Screen Pinning system notifications; using immersive fullscreen kiosk mode instead.");                
                startLockTask();
            }
        } catch (Exception e) {
            Log.e(TAG, "Failed to start LockTask", e);
        }
    }

    public void stopKioskLockTask() {
        try {
            int state = getLockTaskModeState();
            if (state != ActivityManager.LOCK_TASK_MODE_NONE) {
                Log.i(TAG, "Stopping LockTask (current state=" + state + ")");
                stopLockTask();
            }
        } catch (Exception e) {
            Log.e(TAG, "Failed to stop LockTask", e);
        }
    }

    public void collapseStatusBar() {
        try {
            @SuppressLint("WrongConstant") Object statusBarService = getSystemService("statusbar");
            if (statusBarService != null) {
                Class<?> statusBarManager = Class.forName("android.app.StatusBarManager");
                try {
                    java.lang.reflect.Method collapse = statusBarManager.getMethod("collapsePanels");
                    collapse.setAccessible(true);
                    collapse.invoke(statusBarService);
                } catch (NoSuchMethodException e) {
                    java.lang.reflect.Method collapse = statusBarManager.getMethod("collapse");
                    collapse.setAccessible(true);
                    collapse.invoke(statusBarService);
                }
            }
        } catch (Exception ignored) {}

        try {
            @SuppressWarnings("deprecation")
            Intent closeDialog = new Intent(Intent.ACTION_CLOSE_SYSTEM_DIALOGS);
            sendBroadcast(closeDialog);
        } catch (Exception ignored) {}
    }

    private void applyImmersiveMode(boolean fullscreen) {
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                WindowInsetsControllerCompat controller = WindowCompat.getInsetsController(getWindow(), getWindow().getDecorView());
                if (controller != null) {
                    if (fullscreen) {
                        controller.hide(WindowInsetsCompat.Type.systemBars());
                        controller.setSystemBarsBehavior(WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE);
                    } else {
                        controller.show(WindowInsetsCompat.Type.systemBars());
                    }
                }
            } else {
                View decorView = getWindow().getDecorView();
                if (fullscreen) {
                    int flags = View.SYSTEM_UI_FLAG_LAYOUT_STABLE
                        | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
                        | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                        | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                        | View.SYSTEM_UI_FLAG_FULLSCREEN
                        | View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY;
                    decorView.setSystemUiVisibility(flags);
                } else {
                    decorView.setSystemUiVisibility(View.SYSTEM_UI_FLAG_VISIBLE);
                }
            }
        } catch (Exception ignored) {}
    }

    public boolean isDefaultLauncher() {
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                android.app.role.RoleManager roleManager = getSystemService(android.app.role.RoleManager.class);
                if (roleManager != null && roleManager.isRoleAvailable(android.app.role.RoleManager.ROLE_HOME)) {
                    return roleManager.isRoleHeld(android.app.role.RoleManager.ROLE_HOME);
                }
            }
            Intent intent = new Intent(Intent.ACTION_MAIN);
            intent.addCategory(Intent.CATEGORY_HOME);
            android.content.pm.ResolveInfo resolveInfo = getPackageManager().resolveActivity(intent, PackageManager.MATCH_DEFAULT_ONLY);
            if (resolveInfo != null && resolveInfo.activityInfo != null) {
                return getPackageName().equals(resolveInfo.activityInfo.packageName);
            }
        } catch (Exception e) {
            Log.w(TAG, "Error checking default launcher: " + e.getMessage());
        }
        return false;
    }

    public void requestDefaultLauncher() {
        runOnUiThread(() -> {
            try {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                    android.app.role.RoleManager roleManager = getSystemService(android.app.role.RoleManager.class);
                    if (roleManager != null && roleManager.isRoleAvailable(android.app.role.RoleManager.ROLE_HOME)) {
                        if (!roleManager.isRoleHeld(android.app.role.RoleManager.ROLE_HOME)) {
                            Intent intent = roleManager.createRequestRoleIntent(android.app.role.RoleManager.ROLE_HOME);
                            startActivityForResult(intent, 7020);
                            return;
                        } else {
                            return;
                        }
                    }
                }
            } catch (Exception e) {
                Log.w(TAG, "RoleManager request failed: " + e.getMessage());
            }

            try {
                Intent intent = new Intent(android.provider.Settings.ACTION_HOME_SETTINGS);
                intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
                startActivity(intent);
            } catch (Exception e) {
                try {
                    Intent intent = new Intent(android.provider.Settings.ACTION_MANAGE_DEFAULT_APPS_SETTINGS);
                    intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
                    startActivity(intent);
                } catch (Exception e2) {
                    try {
                        Intent intent = new Intent(android.provider.Settings.ACTION_SETTINGS);
                        intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
                        startActivity(intent);
                    } catch (Exception ignored) {}
                }
            }
        });
    }

    public void openHomeSettings() {
        runOnUiThread(() -> {
            try {
                Intent intent = new Intent(android.provider.Settings.ACTION_HOME_SETTINGS);
                startActivity(intent);
            } catch (Exception e) {
                try {
                    Intent intent = new Intent(android.provider.Settings.ACTION_SETTINGS);
                    startActivity(intent);
                } catch (Exception ignored) {}
            }
        });
    }

    public void quitAppWithLauncherPrompt() {
        runOnUiThread(() -> {
            try {
                try {
                    getPackageManager().clearPackagePreferredActivities(getPackageName());
                } catch (Exception ignored) {}

                // Open Home settings so user can choose a new default home app upon quitting
                Intent intent = new Intent(android.provider.Settings.ACTION_HOME_SETTINGS);
                intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
                startActivity(intent);

                if (bridge != null) {
                    bridge.stopForegroundService();
                }
                finishAndRemoveTask();
            } catch (Exception e) {
                Log.e(TAG, "Error during quitAppWithLauncherPrompt", e);
                finishAndRemoveTask();
            }
        });
    }

    private long lastCornerTapTime = 0;
    private int cornerTapCount = 0;
    private volatile boolean isWebappActive = false;

    @Override
    public boolean dispatchTouchEvent(MotionEvent ev) {
        if (ev.getAction() == MotionEvent.ACTION_DOWN) {
            String currentUrl = webView != null ? webView.getUrl() : null;
            boolean isExternal = isExternalWebapp(currentUrl);
            if (isKioskFullscreen || isExternal || isWebappActive) {
                float x = ev.getX();
                float y = ev.getY();
                int width = getResources().getDisplayMetrics().widthPixels;
                int height = getResources().getDisplayMetrics().heightPixels;
                float radiusPx = 80 * getResources().getDisplayMetrics().density;
                boolean isCorner = (x <= radiusPx && y <= radiusPx) // Top-Left
                                || (x >= width - radiusPx && y <= radiusPx) // Top-Right
                                || (x <= radiusPx && y >= height - radiusPx) // Bottom-Left
                                || (x >= width - radiusPx && y >= height - radiusPx); // Bottom-Right
                if (isCorner) {
                    long now = System.currentTimeMillis();
                    if (now - lastCornerTapTime > 1200) {
                        cornerTapCount = 0;
                    }
                    lastCornerTapTime = now;
                    cornerTapCount++;
                    if (cornerTapCount >= 4) {
                        cornerTapCount = 0;
                        promptPINToCloseWebapp();
                    }
                }
            }
        }
        return super.dispatchTouchEvent(ev);
    }

    private void promptPINToCloseWebapp() {
        new Thread(() -> {
            boolean hasPIN = false;
            try {
                java.net.URL cfgUrl = new java.net.URL("http://127.0.0.1:4545/api/webview");
                java.net.HttpURLConnection conn = (java.net.HttpURLConnection) cfgUrl.openConnection();
                conn.setConnectTimeout(1000);
                conn.setReadTimeout(1000);
                if (conn.getResponseCode() == 200) {
                    java.io.InputStream is = conn.getInputStream();
                    java.util.Scanner s = new java.util.Scanner(is).useDelimiter("\\A");
                    String resp = s.hasNext() ? s.next() : "";
                    org.json.JSONObject obj = new org.json.JSONObject(resp);
                    hasPIN = obj.optBoolean("hasPIN", false);
                }
            } catch (Exception ignored) {}

            if (!hasPIN) {
                verifyPinAndExit("");
                return;
            }

            runOnUiThread(() -> {
                final android.widget.EditText pinInput = new android.widget.EditText(this);
                pinInput.setInputType(android.text.InputType.TYPE_CLASS_NUMBER | android.text.InputType.TYPE_NUMBER_VARIATION_PASSWORD);
                pinInput.setHint("Enter Admin PIN");
                pinInput.setPadding(60, 40, 60, 40);

                new androidx.appcompat.app.AlertDialog.Builder(this)
                    .setTitle("Close Web Application")
                    .setMessage("Enter the administrator PIN to close the web app and return to settings:")
                    .setView(pinInput)
                    .setPositiveButton("Unlock & Exit", (dialog, which) -> {
                        String enteredPin = pinInput.getText().toString().trim();
                        verifyPinAndExit(enteredPin);
                    })
                    .setNegativeButton("Cancel", (dialog, which) -> dialog.dismiss())
                    .setCancelable(true)
                    .show();
            });
        }).start();
    }

    private void verifyPinAndExit(String pin) {
        new Thread(() -> {
            boolean valid = false;
            try {
                java.net.URL url = new java.net.URL("http://127.0.0.1:4545/api/auth/session");
                java.net.HttpURLConnection conn = (java.net.HttpURLConnection) url.openConnection();
                conn.setRequestMethod("POST");
                conn.setRequestProperty("Content-Type", "application/json");
                conn.setDoOutput(true);
                conn.setConnectTimeout(2000);
                conn.setReadTimeout(2000);
                String jsonBody = "{\"pin\":\"" + pin.replace("\"", "\\\"") + "\"}";
                try (java.io.OutputStream os = conn.getOutputStream()) {
                    os.write(jsonBody.getBytes(java.nio.charset.StandardCharsets.UTF_8));
                }
                int code = conn.getResponseCode();
                valid = (code == 200);
            } catch (Exception e) {
                Log.e(TAG, "PIN check error", e);
                // Fallback to exit if backend not reachable
                valid = true;
            }

            final boolean isSuccess = valid;
            if (isSuccess) {
                try {
                    java.net.URL closeUrl = new java.net.URL("http://127.0.0.1:4545/api/webview/close");
                    java.net.HttpURLConnection closeConn = (java.net.HttpURLConnection) closeUrl.openConnection();
                    closeConn.setRequestMethod("POST");
                    closeConn.setRequestProperty("Content-Type", "application/json");
                    closeConn.setConnectTimeout(2000);
                    closeConn.getResponseCode();
                } catch (Exception ignored) {}
            }
            runOnUiThread(() -> {
                if (isSuccess) {
                    isWebappActive = false;
                    setFullscreenMode(false);
                    if (webView != null) {
                        webView.setInitialScale(0);
                        webView.evaluateJavascript("document.documentElement.style.zoom = '1.0';", null);
                        webView.loadUrl(WAILS_SCHEME + "://" + WAILS_HOST + "/");
                    }
                } else {
                    android.widget.Toast.makeText(this, "Incorrect PIN", android.widget.Toast.LENGTH_SHORT).show();
                    promptPINToCloseWebapp();
                }
            });
        }).start();
    }

    @Override
    public boolean dispatchKeyEvent(KeyEvent event) {
        if (isKioskFullscreen) {
            int keyCode = event.getKeyCode();
            if (keyCode == KeyEvent.KEYCODE_BACK ||
                keyCode == KeyEvent.KEYCODE_HOME ||
                keyCode == KeyEvent.KEYCODE_APP_SWITCH) {
                return true;
            }
        }
        return super.dispatchKeyEvent(event);
    }

    @Override
    public void onBackPressed() {
        if (isKioskFullscreen) {
            // Lock back button during kiosk mode
            return;
        }
        if (webView != null && webView.canGoBack()) {
            webView.goBack();
        } else {
            super.onBackPressed();
        }
    }

    private void startWebviewActionPoller() {
        Thread t = new Thread(() -> {
            // Initial check for kiosk auto-launch
            try {
                java.net.URL cfgUrl = new java.net.URL("http://127.0.0.1:4545/api/webview");
                java.net.HttpURLConnection cfgConn = (java.net.HttpURLConnection) cfgUrl.openConnection();
                cfgConn.setConnectTimeout(1500);
                cfgConn.setReadTimeout(1500);
                if (cfgConn.getResponseCode() == 200) {
                    java.io.InputStream is = cfgConn.getInputStream();
                    java.util.Scanner s = new java.util.Scanner(is).useDelimiter("\\A");
                    String resp = s.hasNext() ? s.next() : "";
                    org.json.JSONObject obj = new org.json.JSONObject(resp);
                    String startupUrl = obj.optString("url", "");
                    boolean enabled = obj.optBoolean("enabled", false);
                    double zoom = obj.optDouble("zoom", 1.0);
                    if (!startupUrl.isEmpty() && enabled) {
                        if (zoom > 0) currentWebappZoom = zoom;
                        isWebappActive = true;
                        runOnUiThread(() -> {
                            setFullscreenMode(true);
                            if (webView != null) {
                                webView.setInitialScale(0);
                                webView.loadUrl(startupUrl);
                            }
                        });
                    }
                }
            } catch (Exception ignored) {}

            long lastId = 0;
            while (!isDestroyed() && !isFinishing()) {
                try {
                    java.net.URL url = new java.net.URL("http://127.0.0.1:4545/api/webview/poll-action?lastId=" + lastId);
                    java.net.HttpURLConnection conn = (java.net.HttpURLConnection) url.openConnection();
                    conn.setConnectTimeout(5000);
                    conn.setReadTimeout(30000);
                    if (conn.getResponseCode() == 200) {
                        java.io.InputStream is = conn.getInputStream();
                        java.util.Scanner s = new java.util.Scanner(is).useDelimiter("\\A");
                        String resp = s.hasNext() ? s.next() : "";
                        org.json.JSONObject obj = new org.json.JSONObject(resp);
                        long id = obj.optLong("id", lastId);
                        String action = obj.optString("action", "none");
                        if (id > lastId) {
                            lastId = id;
                            if ("open".equals(action)) {
                                String targetUrl = obj.optString("url", "");
                                boolean fs = obj.optBoolean("fullscreen", false);
                                double openZoom = obj.optDouble("zoom", 1.0);
                                if (openZoom > 0) {
                                    currentWebappZoom = openZoom;
                                }
                                isWebappActive = true;
                                runOnUiThread(() -> {
                                    if (fs) {
                                        setFullscreenMode(true);
                                    } else {
                                        setFullscreenMode(false);
                                    }
                                    if (webView != null && !targetUrl.isEmpty()) {
                                        webView.setInitialScale(0);
                                        webView.loadUrl(targetUrl);
                                    }
                                });
                            } else if ("close".equals(action)) {
                                isWebappActive = false;
                                runOnUiThread(() -> {
                                    setFullscreenMode(false);
                                    if (webView != null) {
                                        webView.setInitialScale(0);
                                        webView.evaluateJavascript("document.documentElement.style.zoom = '1.0';", null);
                                        webView.loadUrl(WAILS_SCHEME + "://" + WAILS_HOST + "/");
                                    }
                                });
                            } else if ("reload".equals(action)) {
                                runOnUiThread(() -> {
                                    if (webView != null) {
                                        webView.reload();
                                    }
                                });
                            } else if ("lockdown".equals(action)) {
                                boolean fs = obj.optBoolean("fullscreen", false);
                                runOnUiThread(() -> {
                                    setFullscreenMode(fs);
                                });
                            } else if ("zoom".equals(action)) {
                                double zoomLevel = obj.optDouble("zoom", 1.0);
                                applyWebViewZoom(zoomLevel);
                            }
                        }
                    } else {
                        Thread.sleep(1000);
                    }
                } catch (Exception e) {
                    try { Thread.sleep(1000); } catch (Exception ignored) {}
                }
            }
        });
        t.setName("WebviewActionPoller");
        t.setDaemon(true);
        t.start();
    }
}
