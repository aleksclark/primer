import { useSyncExternalStore } from "react";
import { authSnapshot, subscribeAuth, type AuthState } from "@/api/auth";

/** useAuth subscribes the SPA shell to parent session token changes. */
export function useAuth(): AuthState {
  return useSyncExternalStore(subscribeAuth, authSnapshot, authSnapshot);
}
