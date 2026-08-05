import { useState } from "react";
import { KeyRound, ShieldAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { clearParentToken, setParentToken } from "@/api/auth";
import { useAuth } from "@/hooks/use-auth";

/**
 * ParentTokenGate collects a parent session Bearer token before student-client
 * pages call parent-guarded APIs. Generic CRUD resources still work without it
 * on open local servers; student-client routes require a token.
 */
export function ParentTokenGate({ children }: { children: React.ReactNode }) {
  const { token, rejected } = useAuth();
  const [skipped, setSkipped] = useState(false);

  if (rejected) {
    return (
      <TokenForm
        rejected
        onForget={() => {
          setSkipped(false);
          clearParentToken();
        }}
      />
    );
  }

  if (token || skipped) return <>{children}</>;

  return <TokenForm rejected={false} onSkip={() => setSkipped(true)} />;
}

interface TokenFormProps {
  rejected: boolean;
  onSkip?: () => void;
  onForget?: () => void;
}

function TokenForm({ rejected, onSkip, onForget }: TokenFormProps) {
  const [value, setValue] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const login = async () => {
    setBusy(true);
    setError(null);
    try {
      const res = await fetch("/api/v1/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error((body as { detail?: string }).detail ?? `HTTP ${res.status}`);
      }
      const body = (await res.json()) as { token?: string };
      if (!body.token) throw new Error("login response missing token");
      setParentToken(body.token);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-[50vh] items-center justify-center p-6">
      <form
        className="w-full max-w-sm space-y-4 border border-border bg-surface-raised p-6"
        onSubmit={(e) => {
          e.preventDefault();
          if (email && password) {
            void login();
            return;
          }
          if (value) setParentToken(value);
        }}
      >
        <div className="space-y-2 border-b border-border pb-4">
          <p className="type-label text-muted-foreground">Parent session</p>
          <h1 className="type-h3 inline-flex items-center gap-2 text-foreground">
            <KeyRound className="h-5 w-5" /> Student client access
          </h1>
          <p className="text-sm text-muted-foreground">
            Sign in with a parent/admin educator, paste a Bearer token from{" "}
            <code className="font-mono">POST /auth/login</code>, or set{" "}
            <code className="font-mono">VITE_PARENT_TOKEN</code>.
          </p>
        </div>

        {(rejected || error) && (
          <p
            className="type-label inline-flex items-start gap-2 border border-attention px-3 py-2 text-attention"
            role="alert"
          >
            <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" />
            {error ?? "The server rejected that session token. Sign in again."}
          </p>
        )}

        <div className="space-y-2">
          <Label htmlFor="parent-email">Email</Label>
          <Input
            id="parent-email"
            type="email"
            autoComplete="username"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="parent-password">Password</Label>
          <Input
            id="parent-password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>

        <div className="space-y-2 border-t border-border pt-4">
          <Label htmlFor="parent-token">Or paste session token</Label>
          <Input
            id="parent-token"
            type="password"
            autoComplete="off"
            value={value}
            onChange={(e) => setValue(e.target.value)}
          />
        </div>

        <div className="flex flex-col gap-2 border-t border-border pt-4">
          <Button type="submit" disabled={busy || (!email && !password && !value)}>
            {busy ? "Signing in…" : "Continue"}
          </Button>
          {rejected ? (
            <Button type="button" variant="ghost" size="sm" onClick={onForget}>
              Forget stored token
            </Button>
          ) : (
            onSkip && (
              <Button type="button" variant="ghost" size="sm" onClick={onSkip}>
                Continue without a token
              </Button>
            )
          )}
        </div>
      </form>
    </div>
  );
}
