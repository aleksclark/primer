import { useSyncExternalStore } from "react";
import { authSnapshot, subscribeAuth, type AuthState } from "@/api/auth";

/** useAuth subscribes the app shell to the stored admin key and its status. */
export function useAuth(): AuthState {
  return useSyncExternalStore(subscribeAuth, authSnapshot);
}
