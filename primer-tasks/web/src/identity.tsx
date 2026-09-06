import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { ClerkProvider, useAuth, useClerk } from "@clerk/react";
import { useLocation } from "react-router-dom";
import { configureTasksClient, tasksClient } from "@primer-tasks/client";

export const appBase = import.meta.env.BASE_URL;
const apiBase = import.meta.env.VITE_TASKS_API_BASE ?? `${appBase}api`;
const returnURL = `${window.location.origin}${appBase}parent/students`;
const publishableKey = import.meta.env.VITE_CLERK_PUBLISHABLE_KEY;
configureTasksClient({ baseUrl: apiBase });

const Identity = createContext({
  clerk: false,
  ready: true,
  signedIn: false,
  signIn: () => tasksClient.beginParentLogin(`${appBase}parent/students`),
  signOut: async () => { await tasksClient.logout(); window.location.assign(returnURL); },
});
export const useParentIdentity = () => useContext(Identity);

function ClerkSession({ children }: { children: ReactNode }) {
  const { getToken, isLoaded, isSignedIn } = useAuth();
  const clerk = useClerk();
  const [configured, setConfigured] = useState(false);
  useEffect(() => {
    configureTasksClient({ baseUrl: apiBase, getParentToken: () => getToken() });
    setConfigured(true);
  }, [getToken]);
  if (!configured || !isLoaded) return <p role="status">Loading sign-in…</p>;
  return <Identity.Provider value={{
    clerk: true, ready: isLoaded, signedIn: Boolean(isSignedIn),
    signIn: () => clerk.openSignIn({ forceRedirectUrl: returnURL, signUpForceRedirectUrl: returnURL }),
    signOut: async () => {
      // Local sid revocation precedes provider logout. Never claim success if
      // the application could not persist its own immediate revocation.
      await tasksClient.logout();
      await clerk.signOut({ redirectUrl: returnURL });
    },
  }}>{children}</Identity.Provider>;
}

export function TasksIdentity({ children }: { children: ReactNode }) {
  const { pathname } = useLocation();
  // Paired students do not need Clerk to initialize, sign in, or be reachable.
  if (pathname === "/student" || pathname.startsWith("/student/")) return children;
  if (!publishableKey) return children; // P1/P2 development fixture only
  return <ClerkProvider publishableKey={publishableKey} signInForceRedirectUrl={returnURL} signUpForceRedirectUrl={returnURL}>
    <ClerkSession>{children}</ClerkSession>
  </ClerkProvider>;
}
