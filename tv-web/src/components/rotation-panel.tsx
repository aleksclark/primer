import { useCallback, useEffect, useState } from "react";
import { RefreshCw, Shuffle } from "lucide-react";
import type { components } from "@/api/schema";
import { get, mutate } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";

type Suggestion = components["schemas"]["RotationSuggestion"];
type RotateResponse = components["schemas"]["RotateResponse"];

const NEVER_OFFERED_BEFORE = new Date("1971-01-01T00:00:00Z");

/** How long a rotation's windows stay open, in days. */
const SPANS = [7, 14, 30];

/**
 * RotationPanel turns the weekly catalog refresh into one decision instead of
 * a dozen hand-authored windows.
 *
 * The server proposes only items that can actually play and whose viewing has
 * not already been spent, so accepting the suggestion wholesale is a
 * reasonable default rather than something the parent has to audit.
 */
export function RotationPanel({ onRotated }: { onRotated?: () => void }) {
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [days, setDays] = useState(7);
  const [expireOpen, setExpireOpen] = useState(true);
  const [result, setResult] = useState<RotateResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    get<{ suggestions: Suggestion[] }>("/rotation/suggestions?limit=20")
      .then((body) => {
        setSuggestions(body.suggestions ?? []);
        setError(null);
      })
      .catch((err: Error) => setError(err.message));
  }, []);

  useEffect(load, [load]);

  const toggle = (id: string) => {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const rotate = async () => {
    setBusy(true);
    setError(null);
    setResult(null);
    try {
      const body = (await mutate("POST", "/rotation/rotate", {
        mediaItemIds: [...selected],
        days,
        expireOpen,
        limit: 8,
      })) as RotateResponse;
      setResult(body);
      setSelected(new Set());
      load();
      onRotated?.();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="rounded-md border p-4 space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="font-medium">Rotate the catalog</h2>
          <p className="text-xs text-muted-foreground">
            Items that can play, are not currently offered, and have not been watched.
            Selecting none takes the server&apos;s own pick.
          </p>
        </div>
        <Button variant="outline" size="icon" onClick={load} title="Refresh suggestions">
          <RefreshCw />
        </Button>
      </div>

      {error && <p className="text-sm text-destructive">{error}</p>}

      {suggestions.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Nothing is waiting. Import more from the library, or let the current windows close.
        </p>
      ) : (
        <div className="max-h-64 space-y-1 overflow-y-auto">
          {suggestions.map((s) => {
            const lastOffered = new Date(s.lastWindowEndedAt);
            const neverOffered = lastOffered < NEVER_OFFERED_BEFORE;
            return (
              <label
                key={s.mediaItem.id}
                className="flex items-center gap-2 rounded px-2 py-1 text-sm hover:bg-accent"
              >
                <input
                  type="checkbox"
                  className="h-4 w-4"
                  checked={selected.has(s.mediaItem.id)}
                  onChange={() => toggle(s.mediaItem.id)}
                  aria-label={`Offer ${s.mediaItem.title}`}
                />
                <span className="flex-1">{s.mediaItem.title}</span>
                <span className="text-xs text-muted-foreground">{s.mediaItem.class}</span>
                <span className="text-xs text-muted-foreground">
                  {neverOffered ? "never offered" : `last offered ${lastOffered.toLocaleDateString()}`}
                </span>
              </label>
            );
          })}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-4">
        <div className="flex items-center gap-2">
          <Label className="text-sm">Open for</Label>
          {SPANS.map((span) => (
            <Button
              key={span}
              size="sm"
              variant={span === days ? "default" : "outline"}
              onClick={() => setDays(span)}
            >
              {span}d
            </Button>
          ))}
        </div>

        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="h-4 w-4"
            checked={expireOpen}
            onChange={(e) => setExpireOpen(e.target.checked)}
          />
          Close what is currently offered
        </label>

        <Button onClick={rotate} disabled={busy}>
          <Shuffle /> {busy ? "Rotating…" : "Rotate"}
        </Button>
      </div>

      {result && (
        <p className="text-sm text-muted-foreground">
          Closed {result.expired}, opened {result.opened.length}.
        </p>
      )}
    </div>
  );
}
