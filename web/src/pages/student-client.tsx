import { useCallback, useEffect, useMemo, useState } from "react";
import { RefreshCw } from "lucide-react";
import type { components } from "@/api/schema";
import { get, mutate, type Page } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { shortDate, shortId } from "@/lib/columns";
import { cn } from "@/lib/utils";

type StudentDevice = components["schemas"]["StudentDevice"];
type StudentAssignment = components["schemas"]["StudentAssignment"];
type LearningSession = components["schemas"]["LearningSession"];
type Student = components["schemas"]["Student"];
type MasteryRecord = components["schemas"]["MasteryRecord"];

type StudentMetrics = {
  devicesActive: number;
  devicesRevoked: number;
  assignmentsOpen: number;
  sessionsActive: number;
  completionsLast24h: number;
  tutorFailuresLast24h: number;
};

type LearningOverview = {
  student: Student;
  devices: StudentDevice[];
  openAssignments: StudentAssignment[];
  recentSessions: LearningSession[];
  masterySummary: MasteryRecord[];
  tutor: {
    enabled: boolean;
    provider: string;
    recentFailureCount: number;
    studentNotesDisable?: boolean;
  };
  tutorNotesDisable: boolean;
};

function Panel({
  title,
  hint,
  actions,
  children,
}: {
  title: string;
  hint?: string;
  actions?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-3 border border-border bg-surface-raised p-5">
      <div className="flex items-start justify-between gap-3 border-b border-border pb-3">
        <div className="space-y-1">
          <h2 className="type-h3 text-foreground">{title}</h2>
          {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
        </div>
        {actions}
      </div>
      {children}
    </div>
  );
}

function formatWhen(v?: string | null) {
  if (!v) return "—";
  try {
    return new Date(v).toLocaleString();
  } catch {
    return v;
  }
}

/** StudentDevicesPage lists paired workstations with revoke / re-pair actions. */
export function StudentDevicesPage() {
  const [items, setItems] = useState<StudentDevice[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [notice, setNotice] = useState<string | null>(null);
  const [tick, setTick] = useState(0);

  const load = useCallback(() => {
    setLoading(true);
    get<Page<StudentDevice>>("/student-devices")
      .then((page) => {
        setItems(page.items ?? []);
        setError(null);
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
  }, [load, tick]);

  const revoke = async (id: string) => {
    setNotice(null);
    try {
      await mutate("POST", `/student-devices/${id}/revoke`, {});
      setNotice(`Revoked device ${shortId(id)}`);
      setTick((t) => t + 1);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const rotate = async (id: string) => {
    setNotice(null);
    try {
      const body = (await mutate("POST", `/student-devices/${id}/rotate-token`, {})) as {
        code?: string;
        expiresAt?: string;
      };
      setNotice(
        `Device revoked. New pairing code (once): ${body.code ?? "?"} expires ${body.expiresAt ?? "?"}`,
      );
      setTick((t) => t + 1);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="type-label text-muted-foreground">Student client</p>
          <h1 className="type-h2">Devices</h1>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={() => setTick((t) => t + 1)}>
          <RefreshCw className="mr-2 h-3.5 w-3.5" /> Refresh
        </Button>
      </div>
      {error && <p className="text-sm text-attention">{error}</p>}
      {notice && <p className="text-sm text-muted-foreground">{notice}</p>}
      <div className="border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>Name</TableHead>
              <TableHead>Student</TableHead>
              <TableHead>Last seen</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && (
              <TableRow>
                <TableCell colSpan={6} className="text-muted-foreground">
                  Loading…
                </TableCell>
              </TableRow>
            )}
            {!loading && items.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="text-muted-foreground">
                  No devices paired yet.
                </TableCell>
              </TableRow>
            )}
            {items.map((d) => (
              <TableRow key={d.id}>
                <TableCell>
                  <code className="font-mono text-xs">{shortId(d.id)}</code>
                </TableCell>
                <TableCell>{d.name}</TableCell>
                <TableCell>
                  <code className="font-mono text-xs">{shortId(d.studentId)}</code>
                </TableCell>
                <TableCell>{formatWhen(d.lastSeenAt)}</TableCell>
                <TableCell>
                  {d.revokedAt ? (
                    <Badge variant="outline">revoked</Badge>
                  ) : (
                    <Badge>active</Badge>
                  )}
                </TableCell>
                <TableCell className="space-x-2 text-right">
                  {!d.revokedAt && (
                    <Button type="button" size="sm" variant="outline" onClick={() => void revoke(d.id)}>
                      Revoke
                    </Button>
                  )}
                  <Button type="button" size="sm" variant="ghost" onClick={() => void rotate(d.id)}>
                    Rotate / re-pair
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <p className="text-xs text-muted-foreground">
        App version is reported by the workstation broker locally; the LMS device row tracks lastSeenAt
        and revocation. Prefer rotate (new pairing code) over reusing tokens.
      </p>
    </div>
  );
}

/** LearningAssignmentsPage lists household assignments with cancel/retry. */
export function LearningAssignmentsPage() {
  const [items, setItems] = useState<StudentAssignment[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [notice, setNotice] = useState<string | null>(null);
  const [tick, setTick] = useState(0);

  useEffect(() => {
    setLoading(true);
    get<Page<StudentAssignment>>("/assignments?limit=100&sort=created_at&dir=desc")
      .then((page) => {
        setItems(page.items ?? []);
        setError(null);
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false));
  }, [tick]);

  const cancel = async (id: string) => {
    setNotice(null);
    try {
      await mutate("POST", `/assignments/${id}/cancel`, {});
      setNotice(`Cancelled ${shortId(id)}`);
      setTick((t) => t + 1);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const retry = async (id: string) => {
    setNotice(null);
    try {
      const body = (await mutate("POST", `/assignments/${id}/retry`, {
        reason: "parent-retry",
      })) as StudentAssignment;
      setNotice(`Created retry assignment ${shortId(body.id)}`);
      setTick((t) => t + 1);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="type-label text-muted-foreground">Student client</p>
          <h1 className="type-h2">Assignments</h1>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={() => setTick((t) => t + 1)}>
          <RefreshCw className="mr-2 h-3.5 w-3.5" /> Refresh
        </Button>
      </div>
      {error && <p className="text-sm text-attention">{error}</p>}
      {notice && <p className="text-sm text-muted-foreground">{notice}</p>}
      <div className="border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>Student</TableHead>
              <TableHead>State</TableHead>
              <TableHead>Priority</TableHead>
              <TableHead>Reason</TableHead>
              <TableHead>Available</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && (
              <TableRow>
                <TableCell colSpan={7} className="text-muted-foreground">
                  Loading…
                </TableCell>
              </TableRow>
            )}
            {!loading &&
              items.map((a) => (
                <TableRow key={a.id}>
                  <TableCell>
                    <code className="font-mono text-xs">{shortId(a.id)}</code>
                  </TableCell>
                  <TableCell>
                    <code className="font-mono text-xs">{shortId(a.studentId)}</code>
                  </TableCell>
                  <TableCell>
                    <Badge variant={a.state === "cancelled" ? "outline" : "default"}>{a.state}</Badge>
                  </TableCell>
                  <TableCell>{a.priority}</TableCell>
                  <TableCell className="max-w-[12rem] truncate">{a.reason}</TableCell>
                  <TableCell>{shortDate(a.availableAt)}</TableCell>
                  <TableCell className="space-x-2 text-right">
                    {(a.state === "available" || a.state === "in_progress") && (
                      <Button type="button" size="sm" variant="outline" onClick={() => void cancel(a.id)}>
                        Cancel
                      </Button>
                    )}
                    <Button type="button" size="sm" variant="ghost" onClick={() => void retry(a.id)}>
                      Retry
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}

/** LearningSessionsPage lists recent workstation sessions. */
export function LearningSessionsPage() {
  const [items, setItems] = useState<LearningSession[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [tick, setTick] = useState(0);

  useEffect(() => {
    setLoading(true);
    get<Page<LearningSession>>("/learning-sessions?limit=100&sort=started_at&dir=desc")
      .then((page) => {
        setItems(page.items ?? []);
        setError(null);
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false));
  }, [tick]);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="type-label text-muted-foreground">Student client</p>
          <h1 className="type-h2">Sessions</h1>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={() => setTick((t) => t + 1)}>
          <RefreshCw className="mr-2 h-3.5 w-3.5" /> Refresh
        </Button>
      </div>
      {error && <p className="text-sm text-attention">{error}</p>}
      <div className="border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>Student</TableHead>
              <TableHead>State</TableHead>
              <TableHead>Started</TableHead>
              <TableHead>Last event</TableHead>
              <TableHead>Duration</TableHead>
              <TableHead>Summary</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && (
              <TableRow>
                <TableCell colSpan={7} className="text-muted-foreground">
                  Loading…
                </TableCell>
              </TableRow>
            )}
            {!loading && items.length === 0 && (
              <TableRow>
                <TableCell colSpan={7} className="text-muted-foreground">
                  No sessions yet.
                </TableCell>
              </TableRow>
            )}
            {items.map((s) => (
              <TableRow key={s.id}>
                <TableCell>
                  <code className="font-mono text-xs">{shortId(s.id)}</code>
                </TableCell>
                <TableCell>
                  <code className="font-mono text-xs">{shortId(s.studentId)}</code>
                </TableCell>
                <TableCell>
                  <Badge variant="outline">{s.state}</Badge>
                </TableCell>
                <TableCell>{formatWhen(s.startedAt)}</TableCell>
                <TableCell>{formatWhen(s.lastEventAt)}</TableCell>
                <TableCell>{s.durationSeconds ? `${s.durationSeconds}s` : "—"}</TableCell>
                <TableCell className="max-w-[14rem] truncate">{s.summary}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}

/** LearningOverviewPage aggregates one student + household metrics. */
export function LearningOverviewPage() {
  const [students, setStudents] = useState<Student[]>([]);
  const [studentId, setStudentId] = useState("");
  const [overview, setOverview] = useState<LearningOverview | null>(null);
  const [metrics, setMetrics] = useState<StudentMetrics | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [tick, setTick] = useState(0);

  useEffect(() => {
    get<Page<Student>>("/students?limit=100")
      .then((page) => {
        const items = page.items ?? [];
        setStudents(items);
        if (!studentId && items[0]) setStudentId(items[0].id);
      })
      .catch((err: Error) => setError(err.message));
  }, []);

  useEffect(() => {
    get<StudentMetrics>("/ops/student-metrics")
      .then(setMetrics)
      .catch((err: Error) => setError(err.message));
  }, [tick]);

  useEffect(() => {
    if (!studentId) return;
    setLoading(true);
    get<LearningOverview>(`/students/${studentId}/learning-overview`)
      .then((body) => {
        setOverview(body);
        setError(null);
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false));
  }, [studentId, tick]);

  const selectedLabel = useMemo(() => {
    const s = students.find((x) => x.id === studentId);
    return s ? `${s.firstName} ${s.lastName}` : studentId;
  }, [students, studentId]);

  const toggleTutor = async (enabled: boolean) => {
    if (!studentId) return;
    setNotice(null);
    try {
      await mutate("POST", `/students/${studentId}/tutor`, { enabled });
      setNotice(enabled ? "Tutor enabled" : "Tutor disabled (tutor:off)");
      setTick((t) => t + 1);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const assignNext = async () => {
    if (!studentId) return;
    setNotice(null);
    try {
      const body = (await mutate("POST", `/students/${studentId}/assign-next`, {
        slug: "basic-navigation",
      })) as { assignment?: StudentAssignment; reason?: string; created?: boolean };
      setNotice(
        `${body.created ? "Created" : "Existing"} assignment ${shortId(body.assignment?.id)} (${body.reason})`,
      );
      setTick((t) => t + 1);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="type-label text-muted-foreground">Student client</p>
          <h1 className="type-h2">Learning overview</h1>
        </div>
        <div className="flex flex-wrap items-end gap-2">
          <label className="space-y-1 text-sm">
            <span className="type-label text-muted-foreground">Student</span>
            <select
              className={cn(
                "flex h-10 min-w-[14rem] border border-input bg-surface-raised px-3 text-sm",
              )}
              value={studentId}
              onChange={(e) => setStudentId(e.target.value)}
            >
              {students.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.firstName} {s.lastName}
                </option>
              ))}
            </select>
          </label>
          <Button type="button" variant="outline" size="sm" onClick={() => setTick((t) => t + 1)}>
            <RefreshCw className="mr-2 h-3.5 w-3.5" /> Refresh
          </Button>
        </div>
      </div>

      {error && <p className="text-sm text-attention">{error}</p>}
      {notice && <p className="text-sm text-muted-foreground">{notice}</p>}

      {metrics && (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {[
            ["Devices active", metrics.devicesActive],
            ["Devices revoked", metrics.devicesRevoked],
            ["Assignments open", metrics.assignmentsOpen],
            ["Sessions active", metrics.sessionsActive],
            ["Completions (24h)", metrics.completionsLast24h],
            ["Tutor failures (proc)", metrics.tutorFailuresLast24h],
          ].map(([label, value]) => (
            <div key={String(label)} className="border border-border bg-surface-raised p-4">
              <p className="type-label text-muted-foreground">{label}</p>
              <p className="type-h2 mt-1">{value}</p>
            </div>
          ))}
        </div>
      )}

      {loading && <p className="text-sm text-muted-foreground">Loading overview for {selectedLabel}…</p>}

      {overview && !loading && (
        <div className="grid gap-4 lg:grid-cols-2">
          <Panel
            title="Tutor"
            hint={`Provider ${overview.tutor.provider}`}
            actions={
              <div className="flex gap-2">
                <Button type="button" size="sm" variant="outline" onClick={() => void toggleTutor(true)}>
                  Enable
                </Button>
                <Button type="button" size="sm" variant="ghost" onClick={() => void toggleTutor(false)}>
                  Disable
                </Button>
              </div>
            }
          >
            <p className="text-sm">
              Status:{" "}
              <Badge variant={overview.tutor.enabled ? "default" : "outline"}>
                {overview.tutor.enabled ? "enabled" : "disabled"}
              </Badge>
              {overview.tutorNotesDisable && (
                <span className="ml-2 text-muted-foreground">notes contain tutor:off</span>
              )}
            </p>
            <p className="mt-2 text-sm text-muted-foreground">
              Recent process failures: {overview.tutor.recentFailureCount}
            </p>
          </Panel>

          <Panel
            title="Work queue"
            actions={
              <Button type="button" size="sm" onClick={() => void assignNext()}>
                Assign basic-navigation
              </Button>
            }
          >
            {overview.openAssignments.length === 0 ? (
              <p className="text-sm text-muted-foreground">No open assignments.</p>
            ) : (
              <ul className="space-y-2 text-sm">
                {overview.openAssignments.map((a) => (
                  <li key={a.id} className="flex justify-between gap-2 border-b border-border pb-2">
                    <span>
                      <code className="font-mono text-xs">{shortId(a.id)}</code> · {a.state}
                    </span>
                    <span className="text-muted-foreground">p{a.priority}</span>
                  </li>
                ))}
              </ul>
            )}
          </Panel>

          <Panel title="Devices">
            {overview.devices.length === 0 ? (
              <p className="text-sm text-muted-foreground">No devices.</p>
            ) : (
              <ul className="space-y-2 text-sm">
                {overview.devices.map((d) => (
                  <li key={d.id} className="flex justify-between gap-2">
                    <span>
                      {d.name}{" "}
                      <code className="font-mono text-xs text-muted-foreground">{shortId(d.id)}</code>
                    </span>
                    <span className="text-muted-foreground">
                      {d.revokedAt ? "revoked" : formatWhen(d.lastSeenAt)}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </Panel>

          <Panel title="Recent sessions">
            {overview.recentSessions.length === 0 ? (
              <p className="text-sm text-muted-foreground">No sessions.</p>
            ) : (
              <ul className="space-y-2 text-sm">
                {overview.recentSessions.slice(0, 8).map((s) => (
                  <li key={s.id} className="flex justify-between gap-2">
                    <span>
                      <code className="font-mono text-xs">{shortId(s.id)}</code> · {s.state}
                    </span>
                    <span className="text-muted-foreground">{formatWhen(s.startedAt)}</span>
                  </li>
                ))}
              </ul>
            )}
          </Panel>

          <Panel title="Mastery summary" hint="Server-derived records only">
            {overview.masterySummary.length === 0 ? (
              <p className="text-sm text-muted-foreground">No mastery records yet.</p>
            ) : (
              <ul className="space-y-2 text-sm">
                {overview.masterySummary.slice(0, 12).map((m) => (
                  <li key={m.id} className="flex justify-between gap-2">
                    <span>
                      <code className="font-mono text-xs">{shortId(m.standardId)}</code> · {m.status}
                    </span>
                    <span className="text-muted-foreground">{Math.round(m.confidence * 100)}%</span>
                  </li>
                ))}
              </ul>
            )}
          </Panel>

          <Panel title="Pairing" hint="Issue a one-time code for this student">
            <PairingForm studentId={studentId} onIssued={(msg) => setNotice(msg)} onError={setError} />
          </Panel>
        </div>
      )}
    </div>
  );
}

function PairingForm({
  studentId,
  onIssued,
  onError,
}: {
  studentId: string;
  onIssued: (msg: string) => void;
  onError: (msg: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [code, setCode] = useState("");

  return (
    <div className="space-y-3">
      <Button
        type="button"
        size="sm"
        disabled={!studentId || busy}
        onClick={() => {
          setBusy(true);
          mutate("POST", "/pairing-codes", { studentId })
            .then((body) => {
              const b = body as { code?: string; expiresAt?: string };
              setCode(b.code ?? "");
              onIssued(`Pairing code ${b.code} (expires ${b.expiresAt ?? "?"})`);
            })
            .catch((err: Error) => onError(err.message))
            .finally(() => setBusy(false));
        }}
      >
        Issue pairing code
      </Button>
      {code && (
        <div className="space-y-1">
          <p className="type-label text-muted-foreground">Show once on the workstation</p>
          <Input readOnly value={code} className="font-mono tracking-widest" />
        </div>
      )}
    </div>
  );
}
