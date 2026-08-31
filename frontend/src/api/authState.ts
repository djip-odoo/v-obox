/**
 * authState — module-level singleton that holds the current auth credentials.
 *
 * RuntimeContext sets the Wails token once on startup.
 * PIN sessions set the session token in sessionStorage (persists across renders
 * but clears on tab/page refresh as the user requested).
 *
 * The API client calls `getAuthHeaders()` for every privileged request.
 */

const SESSION_KEY = "epos-pin-session-token";

let _wailsToken: string | null = null;

/** Called once by RuntimeContext when the Wails token is obtained. */
export function setWailsToken(token: string): void {
  _wailsToken = token;
}

/** Returns true when running inside the trusted Wails application. */
export function isWailsContext(): boolean {
  return _wailsToken !== null;
}

/** Retrieves the active PIN session token from sessionStorage. */
export function getPINSessionToken(): string | null {
  return sessionStorage.getItem(SESSION_KEY);
}

/** Persists a PIN session token (until tab refresh). */
export function setPINSessionToken(token: string): void {
  sessionStorage.setItem(SESSION_KEY, token);
}

/** Clears the PIN session token (called on 401 responses). */
export function clearPINSessionToken(): void {
  sessionStorage.removeItem(SESSION_KEY);
}

/**
 * Returns the HTTP headers needed to authenticate a privileged request:
 *   - Wails context  → X-Wails-Token
 *   - Remote context → Authorization: Bearer <pin-session-token>
 */
export function getAuthHeaders(): Record<string, string> {
  if (_wailsToken) {
    return { "X-Wails-Token": _wailsToken };
  }
  const session = getPINSessionToken();
  if (session) {
    return { Authorization: `Bearer ${session}` };
  }
  return {};
}
