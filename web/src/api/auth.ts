/**
 * Parent session token store for the LMS admin SPA.
 *
 * Parent-guarded routes expect Authorization: Bearer <token> from
 * POST /auth/login. The token is kept in localStorage so a refresh keeps
 * the session; VITE_PARENT_TOKEN can seed it for local development.
 */

const STORAGE_KEY = "primer-parent-token";

export interface AuthState {
  token: string;
  rejected: boolean;
}

function initialToken(): string {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) return stored;
  } catch {
    // private mode
  }
  const env = (import.meta as { env?: { VITE_PARENT_TOKEN?: string } }).env?.VITE_PARENT_TOKEN;
  return env ?? "";
}

let state: AuthState = { token: initialToken(), rejected: false };

const listeners = new Set<() => void>();

function publish(next: AuthState) {
  state = next;
  for (const listener of listeners) listener();
}

export function authSnapshot(): AuthState {
  return state;
}

export function subscribeAuth(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function setParentToken(token: string) {
  try {
    localStorage.setItem(STORAGE_KEY, token);
  } catch {
    // ignore
  }
  publish({ token, rejected: false });
}

export function clearParentToken() {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // ignore
  }
  publish({ token: "", rejected: false });
}

export function reportUnauthorized() {
  if (!state.rejected) publish({ ...state, rejected: true });
}

/** Headers for authenticated parent requests. */
export function authHeaders(): Record<string, string> {
  return state.token ? { Authorization: `Bearer ${state.token}` } : {};
}
