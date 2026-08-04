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
    <div className="flex min-h-screen items-center justify-center bg-background p-6 text-foreground">
      <form
        className="w-full max-w-sm space-y-4 border border-border bg-surface-raised p-6"
        onSubmit={(e) => {
          e.preventDefault();
          if (value) setAdminKey(value);
        }}
      >
        <div className="space-y-2 border-b border-border pb-4">
          <p className="type-label text-muted-foreground">Access</p>
          <h1 className="type-h3 inline-flex items-center gap-2 text-foreground">
            <KeyRound className="h-5 w-5" /> Primer TV admin
          </h1>
          <p className="text-sm text-muted-foreground">
            Enter the admin API key configured as <code className="font-mono">TV_ADMIN_API_KEY</code> on
            the server.
          </p>
        </div>

        {rejected && (
          <p
            className="type-label inline-flex items-start gap-2 border border-attention px-3 py-2 text-attention"
            role="alert"
          >
            <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" />
            The server rejected that key. Check it and try again.
          </p>
        )}

        <div className="space-y-2">
          <Label htmlFor="admin-key">Admin API key</Label>
          <Input
            id="admin-key"
            type="password"
            autoFocus
            autoComplete="off"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            aria-invalid={rejected || undefined}
          />
        </div>

        <div className="flex flex-col gap-2 border-t border-border pt-4">
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
