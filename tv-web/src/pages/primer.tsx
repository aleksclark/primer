import { useState } from "react";
import { Send } from "lucide-react";
import type { components } from "@/api/schema";
import { ResourcePage } from "@/components/resource-page";
import { Button } from "@/components/ui/button";
import { mutate } from "@/api/client";
import { codeCol, dateTimeCol, shortIdCol } from "@/lib/columns";

type PrimerReport = components["schemas"]["PrimerReport"];
type RunSummary = components["schemas"]["PrimerRunResponse"];

/**
 * PrimerPage is the instructional-hours export log: one row per viewing the
 * LMS has been told about. It is what the parent reads to answer "did that
 * documentary count?", and the unique session key behind it is what stops the
 * same viewing being counted twice.
 *
 * Reporting normally runs on a timer inside the TV server; the button forces a
 * pass now. Deleting a row makes its viewing eligible again, which is how a
 * session exported to the wrong LMS gets re-sent.
 */
export function PrimerPage() {
  const [summary, setSummary] = useState<RunSummary | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [running, setRunning] = useState(false);
  const [reloads, setReloads] = useState(0);

  const run = async () => {
    setRunning(true);
    setError(null);
    setSummary(null);
    try {
      setSummary((await mutate("POST", "/primer-reports/run")) as RunSummary);
      setReloads((n) => n + 1);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setRunning(false);
    }
  };

  return (
    <div className="space-y-4">
      <ResourcePage<PrimerReport>
        refreshToken={reloads}
        title="Primer Reports"
        path="/primer-reports"
        description="Viewings already counted as instructional time in the Primer LMS. Only educational and mixed programmes are reported; entertainment never is."
        defaultSort="reported_at"
        defaultDir="desc"
        canCreate={false}
        canEdit={false}
        searchable={false}
        toolbar={
          <Button variant="outline" onClick={() => void run()} disabled={running}>
            <Send /> {running ? "Reporting…" : "Report now"}
          </Button>
        }
        columns={[
          dateTimeCol<PrimerReport>("reported_at", "Reported", (r) => r.reportedAt, { sortable: true }),
          shortIdCol<PrimerReport>("playbackSessionId", "Session", (r) => r.playbackSessionId),
          codeCol<PrimerReport>("primerRef", "Primer log", (r) => r.primerRef),
        ]}
        fields={[]}
      />

      {error && (
        <p className="type-label border border-attention px-3 py-2 text-attention" role="alert">
          {error}
        </p>
      )}
      {summary && <RunSummaryLine summary={summary} />}
    </div>
  );
}

/** RunSummaryLine explains what a forced reporting pass did. */
function RunSummaryLine({ summary }: { summary: RunSummary }) {
  if (summary.scanned === 0) {
    return <p className="type-label text-muted-foreground">Nothing new to report.</p>;
  }
  return (
    <p className="type-label text-muted-foreground">
      Scanned {summary.scanned}: {summary.reported} counted, {summary.duplicate} already known,{" "}
      {summary.failed} left queued.
    </p>
  );
}
