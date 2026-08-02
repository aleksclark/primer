import { useState } from "react";
import { KeyRound, ShieldAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { clearAdminKey, setAdminKey } from "@/api/auth";
import { useAuth } from "@/hooks/use-auth";

/**
 * AdminKeyGate collects the TV admin API key before letting the app talk to
 * the server, and takes over the screen again if the server rejects it.
 *
 * The key is a shared secret (TV_ADMIN_API_KEY), so it is stored in
 * localStorage and sent as X-Admin-Key on every request. A server with no key
 * configured accepts an empty one, hence the explicit "continue without a key"
 * path for local development.
 */
export function AdminKeyGate({ children }: { children: React.ReactNode }) {
  const { key, rejected } = useAuth();
  const [skipped, setSkipped] = useState(false);

  // A rejection always wins: it takes over the screen and cancels a previous
  // "continue without a key", so dismissing it cannot drop straight back into
  // an app that will only be rejected again.
  if (rejected) {
    return (
      <KeyForm
        rejected
        onForget={() => {
          setSkipped(false);
          clearAdminKey();
        }}
      />
    );
  }

  if (key || skipped) return <>{children}</>;

  return <KeyForm rejected={false} onSkip={() => setSkipped(true)} />;
}

interface KeyFormProps {
  rejected: boolean;
  onSkip?: () => void;
  onForget?: () => void;
}

/** KeyForm is the full-screen key entry and unauthorized state. */
function KeyForm({ rejected, onSkip, onForget }: KeyFormProps) {
  const [value, setValue] = useState("");

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <form
        className="w-full max-w-sm space-y-4 rounded-lg border p-6 shadow-sm"
        onSubmit={(e) => {
          e.preventDefault();
          if (value) setAdminKey(value);
        }}
      >
        <div className="space-y-1.5">
          <h1 className="inline-flex items-center gap-2 text-lg font-semibold tracking-tight">
            <KeyRound className="h-5 w-5" /> Primer TV admin
          </h1>
          <p className="text-sm text-muted-foreground">
            Enter the admin API key configured as <code>TV_ADMIN_API_KEY</code> on the server.
          </p>
        </div>

        {rejected && (
          <p className="inline-flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/5 p-2 text-sm text-destructive">
            <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" />
            The server rejected that key. Check it and try again.
          </p>
        )}

        <div className="space-y-1.5">
          <Label htmlFor="admin-key">Admin API key</Label>
          <Input
            id="admin-key"
            type="password"
            autoFocus
            autoComplete="off"
            value={value}
            onChange={(e) => setValue(e.target.value)}
          />
        </div>

        <div className="flex flex-col gap-2">
          <Button type="submit" disabled={!value}>
            Continue
          </Button>
          {rejected ? (
            <Button type="button" variant="ghost" size="sm" onClick={onForget}>
              Forget stored key
            </Button>
          ) : (
            onSkip && (
              <Button type="button" variant="ghost" size="sm" onClick={onSkip}>
                Continue without a key
              </Button>
            )
          )}
        </div>
      </form>
    </div>
  );
}
