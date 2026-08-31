import { createContext, useContext, useEffect, useState } from "react";
import { GetSessionToken } from "../../wailsjs/go/main/App";
import { setWailsToken } from "../api/authState";

/**
 * RuntimeContext — detects whether the frontend is running inside the trusted
 * Wails desktop application or in a plain remote browser.
 *
 * Detection: Wails injects `window.go` (the Go binding namespace) only when
 * the page is served inside the Wails WebView. A remote browser never has it.
 *
 * In Wails context, we fetch the per-launch session token from the Go backend
 * (via the GetSessionToken binding) and register it with authState so that
 * every privileged HTTP call carries the trusted `X-Wails-Token` header.
 */

export function detectWails(): boolean {
  return typeof window !== "undefined" && !!(window as unknown as Record<string, unknown>)["go"];
}

type RuntimeContextType = {
  /** true when running inside the Wails desktop application */
  isWails: boolean;
  /** false while the Wails session token is being fetched */
  ready: boolean;
  /** The server's base URL for QR-code generation. Remote: window.origin. Wails: null until troubleshoot is fetched. */
  serverURL: string | null;
};

export const RuntimeContext = createContext<RuntimeContextType>({
  isWails: false,
  ready: true,
  serverURL: null,
});

export function useRuntime() {
  return useContext(RuntimeContext);
}

interface RuntimeContextWrapperProps {
  children: React.ReactNode;
}

export function RuntimeContextWrapper({ children }: RuntimeContextWrapperProps) {
  const isWails = detectWails();
  const [ready, setReady] = useState(!isWails); // remote: immediately ready
  const [serverURL, setServerURL] = useState<string | null>(
    isWails ? null : window.location.origin,
  );

  useEffect(() => {
    if (!isWails) return;

    // Fetch the session token from Go and register it with authState.
    // This token is only available inside the Wails process — it is never
    // embedded in the built JS bundle and cannot be obtained by remote users.
    GetSessionToken()
      .then((token) => {
        if (token) {
          setWailsToken(token);
        }
        setReady(true);
      })
      .catch((err) => {
        console.error("Failed to get Wails session token:", err);
        setReady(true); // still mark ready so the UI isn't stuck
      });

    // Compute the server URL for the QR code (Wails context).
    // We read it from the location: Wails serves the webview from its own
    // internal asset server, so window.location is not the Fiber port.
    // We'll derive it lazily from the /api/troubleshoot response instead;
    // set a placeholder for now.
    setServerURL(null); // will be populated by WebViewDialog when needed
  }, [isWails]);

  return (
    <RuntimeContext.Provider value={{ isWails, ready, serverURL }}>
      {children}
    </RuntimeContext.Provider>
  );
}
