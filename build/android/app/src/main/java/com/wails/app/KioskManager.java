package com.wails.app;

import android.app.Activity;
import android.app.ActivityManager;
import android.app.admin.DevicePolicyManager;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.content.SharedPreferences;
import android.os.BatteryManager;
import android.os.Build;
import android.os.UserManager;
import android.provider.Settings;
import android.util.Log;
import android.view.View;
import android.view.WindowManager;

import androidx.core.view.WindowCompat;
import androidx.core.view.WindowInsetsCompat;
import androidx.core.view.WindowInsetsControllerCompat;

/**
 * KioskManager manages Android Device Owner policies, Lock Task Mode configuration,
 * and system UI lockdown for a dedicated enterprise kiosk environment.
 */
public class KioskManager {
    private static final String TAG = "KioskManager";
    private static final String PREF_NAME = "epos_kiosk_prefs";
    private static final String PREF_KEY_KIOSK_ENABLED = "kiosk_enabled";

    private static volatile KioskManager sInstance;

    private final Context mContext;
    private final DevicePolicyManager mDpm;
    private final ComponentName mAdminComponent;
    private final SharedPreferences mPrefs;

    private KioskManager(Context context) {
        mContext = context.getApplicationContext();
        mDpm = (DevicePolicyManager) mContext.getSystemService(Context.DEVICE_POLICY_SERVICE);
        mAdminComponent = KioskDeviceAdminReceiver.getComponentName(mContext);
        mPrefs = mContext.getSharedPreferences(PREF_NAME, Context.MODE_PRIVATE);
    }

    public static KioskManager getInstance(Context context) {
        if (sInstance == null) {
            synchronized (KioskManager.class) {
                if (sInstance == null) {
                    sInstance = new KioskManager(context);
                }
            }
        }
        return sInstance;
    }

    public ComponentName getAdminComponent() {
        return mAdminComponent;
    }

    /**
     * Checks if this application is currently the provisioned Device Owner.
     */
    public boolean isDeviceOwner() {
        if (mDpm == null) return false;
        try {
            return mDpm.isDeviceOwnerApp(mContext.getPackageName());
        } catch (Exception e) {
            Log.w(TAG, "Error checking isDeviceOwnerApp: " + e.getMessage());
            return false;
        }
    }

    /**
     * Checks if device admin is active.
     */
    public boolean isAdminActive() {
        if (mDpm == null) return false;
        try {
            return mDpm.isAdminActive(mAdminComponent);
        } catch (Exception e) {
            Log.w(TAG, "Error checking isAdminActive: " + e.getMessage());
            return false;
        }
    }

    /**
     * Checks if Lock Task Mode is permitted for this application.
     */
    public boolean isLockTaskPermitted() {
        if (mDpm == null) return false;
        try {
            return mDpm.isLockTaskPermitted(mContext.getPackageName());
        } catch (Exception e) {
            Log.w(TAG, "Error checking isLockTaskPermitted: " + e.getMessage());
            return false;
        }
    }

    /**
     * Checks if the device is currently in Lock Task Mode.
     */
    public boolean isLockTaskActive() {
        ActivityManager am = (ActivityManager) mContext.getSystemService(Context.ACTIVITY_SERVICE);
        if (am == null) return false;
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            int state = am.getLockTaskModeState();
            return state == ActivityManager.LOCK_TASK_MODE_LOCKED ||
                   state == ActivityManager.LOCK_TASK_MODE_PINNED;
        } else {
            return am.isInLockTaskMode();
        }
    }

    /**
     * Applies full Device Owner policies required for a dedicated kiosk device.
     */
    public void setupKioskPolicies() {
        if (!isDeviceOwner()) {
            Log.d(TAG, "Not device owner; skipping Device Owner policy configuration");
            return;
        }

        try {
            // 1. Whitelist this package for Lock Task Mode
            mDpm.setLockTaskPackages(mAdminComponent, new String[]{mContext.getPackageName()});
            Log.i(TAG, "Lock Task package set: " + mContext.getPackageName());

            // 2. Disable system UI in Lock Task Mode (No Home, No Recents/Overview, No Notifications)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                mDpm.setLockTaskFeatures(mAdminComponent, DevicePolicyManager.LOCK_TASK_FEATURE_NONE);
                Log.i(TAG, "Lock Task features set to LOCK_TASK_FEATURE_NONE");
            }

            // 3. Disable Keyguard / Lockscreen
            mDpm.setKeyguardDisabled(mAdminComponent, true);

            // 4. Disable Status Bar expansion
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
                mDpm.setStatusBarDisabled(mAdminComponent, true);
            }

            // 5. Apply kiosk user restrictions
            mDpm.addUserRestriction(mAdminComponent, UserManager.DISALLOW_CREATE_WINDOWS);
            mDpm.addUserRestriction(mAdminComponent, UserManager.DISALLOW_SYSTEM_ERROR_DIALOGS);
            mDpm.addUserRestriction(mAdminComponent, UserManager.DISALLOW_SAFE_BOOT);
            mDpm.addUserRestriction(mAdminComponent, UserManager.DISALLOW_FACTORY_RESET);

            // 6. Set persistent preferred Home activity so reboot launches MainActivity
            IntentFilter filter = new IntentFilter(Intent.ACTION_MAIN);
            filter.addCategory(Intent.CATEGORY_HOME);
            filter.addCategory(Intent.CATEGORY_DEFAULT);
            ComponentName activityComponent = new ComponentName(mContext, MainActivity.class);
            mDpm.addPersistentPreferredActivity(mAdminComponent, filter, activityComponent);

            // 7. Prevent device sleep when connected to power
            try {
                mDpm.setGlobalSetting(mAdminComponent, Settings.Global.STAY_ON_WHILE_PLUGGED_IN,
                        String.valueOf(BatteryManager.BATTERY_PLUGGED_AC |
                                       BatteryManager.BATTERY_PLUGGED_USB |
                                       BatteryManager.BATTERY_PLUGGED_WIRELESS));
            } catch (Exception e) {
                Log.w(TAG, "Could not set STAY_ON_WHILE_PLUGGED_IN: " + e.getMessage());
            }

            Log.i(TAG, "Device Owner kiosk policies configured successfully");
        } catch (Exception e) {
            Log.e(TAG, "Failed to apply Device Owner kiosk policies", e);
        }
    }

    /**
     * Clears Device Owner kiosk restrictions when maintenance / full unlock is needed.
     */
    public void clearKioskPolicies() {
        if (!isDeviceOwner()) return;

        try {
            mDpm.clearUserRestriction(mAdminComponent, UserManager.DISALLOW_CREATE_WINDOWS);
            mDpm.clearUserRestriction(mAdminComponent, UserManager.DISALLOW_SYSTEM_ERROR_DIALOGS);
            mDpm.setKeyguardDisabled(mAdminComponent, false);
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
                mDpm.setStatusBarDisabled(mAdminComponent, false);
            }
            mDpm.clearPackagePersistentPreferredActivities(mAdminComponent, mContext.getPackageName());
            Log.i(TAG, "Device Owner kiosk policies cleared");
        } catch (Exception e) {
            Log.e(TAG, "Failed to clear Device Owner kiosk policies", e);
        }
    }

    public boolean isKioskEnabled() {
        return mPrefs.getBoolean(PREF_KEY_KIOSK_ENABLED, false);
    }

    public void setKioskEnabled(boolean enabled) {
        mPrefs.edit().putBoolean(PREF_KEY_KIOSK_ENABLED, enabled).apply();
    }

    /**
     * Activates Kiosk lockdown on the given activity.
     */
    public void startKiosk(Activity activity) {
        setKioskEnabled(true);

        if (isDeviceOwner()) {
            setupKioskPolicies();
        }

        applyImmersiveFullscreen(activity, true);

        // Start Lock Task Mode
        try {
            if (!isLockTaskActive()) {
                activity.startLockTask();
                Log.i(TAG, "startLockTask() invoked successfully");
            }
        } catch (Exception e) {
            Log.w(TAG, "startLockTask warning: " + e.getMessage());
        }
    }

    /**
     * Deactivates Kiosk lockdown on the given activity.
     */
    public void stopKiosk(Activity activity) {
        setKioskEnabled(false);

        try {
            if (isLockTaskActive()) {
                activity.stopLockTask();
                Log.i(TAG, "stopLockTask() invoked successfully");
            }
        } catch (Exception e) {
            Log.w(TAG, "stopLockTask warning: " + e.getMessage());
        }

        applyImmersiveFullscreen(activity, false);
    }

    /**
     * Configures window flags and system UI insets for fullscreen immersive presentation.
     */
    public void applyImmersiveFullscreen(Activity activity, boolean fullscreen) {
        activity.runOnUiThread(() -> {
            try {
                if (fullscreen) {
                    activity.getWindow().addFlags(WindowManager.LayoutParams.FLAG_FULLSCREEN);
                    activity.getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
                    activity.getWindow().addFlags(WindowManager.LayoutParams.FLAG_DISMISS_KEYGUARD);
                    activity.getWindow().addFlags(WindowManager.LayoutParams.FLAG_SHOW_WHEN_LOCKED);
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                        activity.getWindow().getAttributes().layoutInDisplayCutoutMode =
                                WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES;
                    }
                } else {
                    activity.getWindow().clearFlags(WindowManager.LayoutParams.FLAG_FULLSCREEN);
                    activity.getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
                    activity.getWindow().clearFlags(WindowManager.LayoutParams.FLAG_DISMISS_KEYGUARD);
                    activity.getWindow().clearFlags(WindowManager.LayoutParams.FLAG_SHOW_WHEN_LOCKED);
                }

                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                    WindowInsetsControllerCompat controller =
                            WindowCompat.getInsetsController(activity.getWindow(), activity.getWindow().getDecorView());
                    if (controller != null) {
                        if (fullscreen) {
                            controller.hide(WindowInsetsCompat.Type.systemBars());
                            controller.setSystemBarsBehavior(
                                    WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE);
                        } else {
                            controller.show(WindowInsetsCompat.Type.systemBars());
                            controller.setSystemBarsBehavior(
                                    WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE);
                        }
                    }
                } else {
                    View decorView = activity.getWindow().getDecorView();
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
            } catch (Exception e) {
                Log.e(TAG, "Failed to apply immersive fullscreen", e);
            }
        });
    }
}
