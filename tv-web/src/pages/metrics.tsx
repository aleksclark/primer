import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import type { components } from "@/api/schema";
import { get } from "@/api/client";
import { Button } from "@/components/ui/button";

type Metrics = components["schemas"]["MetricsResponse"];

const WINDOWS = [7, 14, 30, 90];

/** Renders seconds the way a parent reads them. */
function duration(seconds: number): string {
  if (seconds <= 0) return "0m";
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.round((seconds % 3600) / 60);
  if (hours > 0 && minutes > 0) return `${hours}h ${minutes}m`;
  if (hours > 0) return `${hours}h`;
  return `${minutes}m`;
}

function Panel({ title, hint, children }: { title: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="space-y-3 border border-border bg-surface-raised p-5">
      <div className="space-y-1 border-b border-border pb-3">
        <h2 className="type-h3 text-foreground">{title}</h2>
        {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
      </div>
      {children}
    </div>
  );
}

/** A labelled proportion bar, which reads faster than a table of seconds. */
function Bar({ label, value, total, detail }: { label: string; value: number; total: number; detail: string }) {
  const pct = total > 0 ? Math.round((value / total) * 100) : 0;
  return (
    <div className="space-y-1.5">
      <div className="flex justify-between gap-3 text-sm">
        <span>{label}</span>
        <span className="type-label text-muted-foreground">{detail}</span>
      </div>
      <div className="h-[3px] bg-border">
        <div className="h-[3px] bg-primary" style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

/**
 * MetricsPage answers the questions a parent actually asks: how much was
 * watched, how much of it counted as instruction, which subjects it touched,
 * and how much of the entertainment ration is gone.
 */
export function MetricsPage() {
  const [days, setDays] = useState(14);
  const [data, setData] = useState<Metrics | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [reloads, setReloads] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    get<Metrics>(`/metrics?days=${days}`)
      .then((body) => {
        if (!cancelled) {
          setData(body);
          setError(null);
        }
      })
      .catch((err: Error) => !cancelled && setError(err.message))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [days, reloads]);

  const totalWatched = data?.byClass.reduce((sum, row) => sum + row.watchedSeconds, 0) ?? 0;
  const instructional = data?.byDay.reduce((sum, row) => sum + row.instructionalWatchedSeconds, 0) ?? 0;
  const completionPct =
    data && data.completion.sessions > 0
      ? Math.round((data.completion.completed / data.completion.sessions) * 100)
      : 0;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4 border-b border-rule-strong pb-4">
        <div className="space-y-1">
          <p className="type-label text-muted-foreground">Reporting</p>
          <h1 className="type-h1 text-foreground">Viewing</h1>
        </div>
        <div className="flex items-center gap-2">
          {WINDOWS.map((option) => (
            <Button
              key={option}
              variant={option === days ? "default" : "outline"}
              size="sm"
              onClick={() => setDays(option)}
            >
              {option}d
            </Button>
          ))}
          <Button variant="outline" size="icon" onClick={() => setReloads((n) => n + 1)} title="Refresh">
            <RefreshCw />
          </Button>
        </div>
      </div>

      {error && (
        <p className="type-label border border-attention px-3 py-2 text-attention" role="alert">
          {error}
        </p>
      )}
      {loading && !data && <p className="text-sm text-muted-foreground">Loading…</p>}

      {data && (
        <>
          <div className="grid gap-4 sm:grid-cols-3">
            <Panel title="Watched">
              <p className="type-h2 text-foreground">{duration(totalWatched)}</p>
              <p className="type-label text-muted-foreground">{data.completion.sessions} viewings</p>
            </Panel>
            <Panel title="Instructional" hint="Educational and mixed only">
              <p className="type-h2 text-foreground">{duration(instructional)}</p>
              <p className="type-label text-muted-foreground">reported to Primer as hours</p>
            </Panel>
            <Panel title="Finished" hint="Viewings watched to the end">
              <p className="type-h2 text-foreground">{completionPct}%</p>
              <p className="type-label text-muted-foreground">
                {data.completion.completed} of {data.completion.sessions}
              </p>
            </Panel>
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            <Panel title="By class">
              {data.byClass.length === 0 ? (
                <p className="text-sm text-muted-foreground">Nothing watched in this window.</p>
              ) : (
                <div className="space-y-3">
                  {data.byClass.map((row) => (
                    <Bar
                      key={row.class}
                      label={row.class}
                      value={row.watchedSeconds}
                      total={totalWatched}
                      detail={`${duration(row.watchedSeconds)} · ${row.sessions} viewings`}
                    />
                  ))}
                </div>
              )}
            </Panel>

            <Panel title="By subject" hint="A title counts under every subject it carries">
              {data.bySubject.length === 0 ? (
                <p className="text-sm text-muted-foreground">No tagged viewing in this window.</p>
              ) : (
                <div className="space-y-3">
                  {data.bySubject.slice(0, 8).map((row) => (
                    <Bar
                      key={row.subject}
                      label={row.subject}
                      value={row.watchedSeconds}
                      total={data.bySubject[0].watchedSeconds}
                      detail={duration(row.watchedSeconds)}
                    />
                  ))}
                </div>
              )}
            </Panel>
          </div>

          <Panel
            title="Entertainment ration"
            hint="Single viewings spent against those offered in this window"
          >
            <p className="text-sm">
              <span className="type-h2 text-foreground">{data.entertainment.playsUsed}</span>
              <span className="text-muted-foreground">
                {" "}
                used of {data.entertainment.windowsOffered} offered
              </span>
            </p>
          </Panel>

          <Panel title="By day" hint={`Calendar days in ${data.timezone}`}>
            {data.byDay.length === 0 ? (
              <p className="text-sm text-muted-foreground">Nothing watched in this window.</p>
            ) : (
              <div className="space-y-2">
                {data.byDay.map((row) => (
                  <Bar
                    key={row.day}
                    label={new Date(`${row.day}T00:00:00`).toLocaleDateString()}
                    value={row.watchedSeconds}
                    total={Math.max(...data.byDay.map((d) => d.watchedSeconds))}
                    detail={`${duration(row.watchedSeconds)} · ${duration(row.instructionalWatchedSeconds)} instructional`}
                  />
                ))}
              </div>
            )}
          </Panel>
        </>
      )}
    </div>
  );
}
