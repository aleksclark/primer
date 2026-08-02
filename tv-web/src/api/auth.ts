/**
 * Admin API key store. The TV admin API is guarded by a shared key
 * (X-Admin-Key, env TV_ADMIN_API_KEY on the server), so the SPA keeps the key
 * the operator typed in localStorage and attaches it to every request.
 *
 * The store also tracks whether the server has rejected that key, so a 401 on
 * any request can push the whole app back to the key-entry screen instead of
 * leaving individual pages showing an unexplained error.
 */

const STORAGE_KEY = "primer-tv-admin-key";

/** AuthState is the snapshot rendered by the app shell. */
export interface AuthState {
  key: string;
  rejected: boolean;
}

let state: AuthState = { key: localStorage.getItem(STORAGE_KEY) ?? "", rejected: false };

const listeners = new Set<() => void>();

/** publish swaps in a new snapshot and notifies subscribers. */
function publish(next: AuthState) {
  state = next;
  for (const listener of listeners) listener();
}

/** authSnapshot returns the current auth state (stable between changes). */
export function authSnapshot(): AuthState {
  return state;
}

/** subscribeAuth registers a listener for auth state changes. */
export function subscribeAuth(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** setAdminKey stores a key and clears any previous rejection. */
export function setAdminKey(key: string) {
  localStorage.setItem(STORAGE_KEY, key);
  publish({ key, rejected: false });
}

/** clearAdminKey forgets the stored key, returning the app to key entry. */
export function clearAdminKey() {
  localStorage.removeItem(STORAGE_KEY);
  publish({ key: "", rejected: false });
}

/** reportUnauthorized records that the server rejected the current key. */
export function reportUnauthorized() {
  if (!state.rejected) publish({ ...state, rejected: true });
}

/**
 * authHeaders returns the admin credential headers for a request. An empty key
 * sends nothing, which is valid against a server with no key configured.
 */
export function authHeaders(): Record<string, string> {
  return state.key ? { "X-Admin-Key": state.key } : {};
}
