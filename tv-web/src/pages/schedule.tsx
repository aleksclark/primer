import { useMemo, useState } from "react";
import { CalendarRange, ChevronLeft, ChevronRight, Copy, Table2, Trash2 } from "lucide-react";
import type { components } from "@/api/schema";
import { mutate } from "@/api/client";
import { ResourcePage } from "@/components/resource-page";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useMediaItems } from "@/hooks/use-media-items";
import { useScheduleGrid, type Programme } from "@/hooks/use-schedule-grid";
import { badgeCol, boolCol, dateTimeCol } from "@/lib/columns";
import { SCHEDULE_BLOCKS } from "@/lib/constants";
import {
  addDays,
  dayKey,
  formatDuration,
  formatTime,
  fromLocalInput,
  minutesIntoDay,
  startOfDay,
  startOfWeek,
  toLocalInput,
} from "@/lib/datetime";

type ScheduleEntry = components["schemas"]["ScheduleEntry"];
type CopyWeekResponse = components["schemas"]["CopyWeekResponse"];

/** DAYS_PER_WEEK is the width of the grid, Monday through Sunday. */
const DAYS_PER_WEEK = 7;

/** MINUTES_PER_DAY is the height of a day column, in grid units. */
const MINUTES_PER_DAY = 24 * 60;

/** GRID_PIXELS_PER_HOUR sets how tall an hour of programming draws. */
const GRID_PIXELS_PER_HOUR = 40;

/** DAY_HOURS labels the hour gutter. */
const DAY_HOURS = Array.from({ length: 24 }, (_, h) => h);

/**
 * SchedulePage is the programmed channel's editor.
 *
 * The week grid is the primary view because the thing a parent needs to see is
 * coverage and collision, and neither is visible in a list of timestamps. Each
 * airing is drawn at its real position and length — the server resolves the
 * end of a slot from the item's runtime — so a gap looks like a gap.
 *
 * The table view stays for precise edits and deletions and is the standard
 * `ResourcePage` over the same resource.
 */
export function SchedulePage() {
  const [view, setView] = useState<"grid" | "table">("grid");
  const media = useMediaItems();

  const toolbar = (
    <Button variant="outline" onClick={() => setView(view === "grid" ? "table" : "grid")}>
      {view === "grid" ? <Table2 /> : <CalendarRange />}
      {view === "grid" ? "Table" : "Week grid"}
    </Button>
  );

  if (view === "grid") {
    return <WeekGrid toolbar={toolbar} />;
  }

  return (
    <ResourcePage<ScheduleEntry>
      title="Schedule"
      path="/schedule-entries"
      description="Programmed channel grid. A slot runs from its air time for the item's runtime; overlapping airings are refused."
      defaultSort="airs_at"
      defaultDir="asc"
      searchable={false}
      toolbar={toolbar}
      columns={[
        {
          key: "mediaItemId",
          header: "Media item",
          render: (row) => <span className="font-medium">{media.title(row.mediaItemId)}</span>,
        },
        dateTimeCol<ScheduleEntry>("airs_at", "Airs at", (r) => r.airsAt, { sortable: true }),
        badgeCol<ScheduleEntry>("block", "Block", (r) => r.block, { sortable: true }),
        boolCol<ScheduleEntry>("join_in_progress", "Join in progress", (r) => r.joinInProgress, {
          trueLabel: "Allowed",
          falseLabel: "From start",
        }),
      ]}
      fields={[
        {
          key: "mediaItemId",
          label: "Media item",
          type: "select",
          choices: media.choices,
          required: true,
        },
        { key: "airsAt", label: "Airs at", type: "datetime", required: true },
        {
          key: "block",
          label: "Block",
          type: "select",
          options: SCHEDULE_BLOCKS,
          help: "Left blank, the server labels the slot from its start time.",
        },
        {
          key: "joinInProgress",
          label: "Join in progress",
          type: "checkbox",
          help: "On: a device tuning in late joins at the server's offset. Off: playback starts from the beginning.",
        },
      ]}
    />
  );
}

interface WeekGridProps {
  toolbar: React.ReactNode;
}

/** WeekGrid draws one week of the channel and edits it in place. */
function WeekGrid({ toolbar }: WeekGridProps) {
  const [weekStart, setWeekStart] = useState(() => startOfWeek(new Date()));
  const [editing, setEditing] = useState<Programme | null>(null);
  const [creatingAt, setCreatingAt] = useState<Date | null>(null);
  const [copying, setCopying] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const from = dayKey(weekStart);
  const { data, loading, error: loadError, refresh } = useScheduleGrid(from, DAYS_PER_WEEK);

  const days = useMemo(
    () => Array.from({ length: DAYS_PER_WEEK }, (_, i) => addDays(weekStart, i)),
    [weekStart],
  );

  const programmes = useMemo(() => data?.programmes ?? [], [data]);

  // The server refuses overlapping writes, but a runtime that changed after an
  // entry was placed can still leave the grid overlapping. Recomputing it here
  // means the parent sees the problem instead of discovering it on air.
  const overlapping = useMemo(() => overlaps(programmes), [programmes]);

  const remove = async (programme: Programme) => {
    if (!confirm(`Remove “${programme.title}” from the grid?`)) return;
    setError(null);
    try {
      await mutate("DELETE", `/schedule-entries/${programme.scheduleEntryId}`);
    } catch (err) {
      setError((err as Error).message);
    }
    refresh();
  };

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Schedule</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            The programmed channel, week of {weekStart.toLocaleDateString()}
            {data && ` · times shown in your browser's zone, bucketed server-side in ${data.timezone}`}
          </p>
        </div>
        <div className="flex items-center gap-2">{toolbar}</div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Button variant="outline" size="sm" onClick={() => setWeekStart(addDays(weekStart, -DAYS_PER_WEEK))}>
          <ChevronLeft /> Previous week
        </Button>
        <Button variant="outline" size="sm" onClick={() => setWeekStart(startOfWeek(new Date()))}>
          This week
        </Button>
        <Button variant="outline" size="sm" onClick={() => setWeekStart(addDays(weekStart, DAYS_PER_WEEK))}>
          Next week <ChevronRight />
        </Button>
        <div className="ml-auto flex items-center gap-2">
          <span className="text-sm text-muted-foreground">
            {programmes.length} airing{programmes.length === 1 ? "" : "s"}
          </span>
          <Button variant="outline" size="sm" onClick={() => setCopying(true)}>
            <Copy /> Copy week
          </Button>
        </div>
      </div>

      {loadError && <p className="text-sm text-destructive">{loadError}</p>}
      {error && <p className="text-sm text-destructive">{error}</p>}
      {overlapping.size > 0 && (
        <p className="rounded-md border border-destructive/50 bg-destructive/10 p-2 text-sm text-destructive">
          {overlapping.size} airing{overlapping.size === 1 ? "" : "s"} overlap another slot. This
          happens when an item's runtime changed after it was scheduled — move or remove the
          highlighted airings so the channel stays a single stream.
        </p>
      )}

      <div className="overflow-x-auto rounded-md border">
        <div className="min-w-[60rem]">
          <div
            className="grid border-b bg-muted/30 text-xs font-medium"
            style={{ gridTemplateColumns: `4rem repeat(${DAYS_PER_WEEK}, minmax(0, 1fr))` }}
          >
            <div className="p-2" />
            {days.map((day) => (
              <div key={dayKey(day)} className="border-l p-2 text-center">
                <div>{day.toLocaleDateString(undefined, { weekday: "short" })}</div>
                <div className="text-muted-foreground">{day.getDate()}</div>
              </div>
            ))}
          </div>

          {loading && !data ? (
            <p className="p-6 text-center text-sm text-muted-foreground">Loading…</p>
          ) : (
            <div
              className="grid"
              style={{ gridTemplateColumns: `4rem repeat(${DAYS_PER_WEEK}, minmax(0, 1fr))` }}
            >
              <HourGutter />
              {days.map((day) => (
                <DayColumn
                  key={dayKey(day)}
                  day={day}
                  programmes={programmes}
                  overlapping={overlapping}
                  onPick={(programme) => setEditing(programme)}
                  onEmptyHour={(at) => setCreatingAt(at)}
                />
              ))}
            </div>
          )}
        </div>
      </div>

      <AiringDialog
        programme={editing}
        createAt={creatingAt}
        onClose={() => {
          setEditing(null);
          setCreatingAt(null);
        }}
        onSaved={() => {
          setEditing(null);
          setCreatingAt(null);
          refresh();
        }}
        onRemove={remove}
      />

      <CopyWeekDialog
        open={copying}
        weekStart={weekStart}
        onClose={() => setCopying(false)}
        onCopied={refresh}
      />
    </div>
  );
}

/** HourGutter labels the vertical axis of the grid. */
function HourGutter() {
  return (
    <div className="relative border-r" style={{ height: `${24 * GRID_PIXELS_PER_HOUR}px` }}>
      {DAY_HOURS.map((hour) => (
        <div
          key={hour}
          className="absolute right-1 -translate-y-1/2 text-[10px] text-muted-foreground"
          style={{ top: `${hour * GRID_PIXELS_PER_HOUR}px` }}
        >
          {hour === 0 ? "" : `${hour}:00`}
        </div>
      ))}
    </div>
  );
}

interface DayColumnProps {
  day: Date;
  programmes: Programme[];
  overlapping: Set<string>;
  onPick: (programme: Programme) => void;
  onEmptyHour: (at: Date) => void;
}

/** DayColumn draws one day's airings positioned by their real air times. */
function DayColumn({ day, programmes, overlapping, onPick, onEmptyHour }: DayColumnProps) {
  const dayStart = startOfDay(day).getTime();
  const dayEnd = addDays(startOfDay(day), 1).getTime();

  const visible = programmes.filter((p) => {
    const starts = new Date(p.airsAt).getTime();
    const ends = new Date(p.endsAt).getTime();
    return ends > dayStart && starts < dayEnd;
  });

  return (
    <div className="relative border-l" style={{ height: `${24 * GRID_PIXELS_PER_HOUR}px` }}>
      {DAY_HOURS.map((hour) => (
        <button
          key={hour}
          type="button"
          className="absolute w-full border-t border-border/40 hover:bg-accent/40"
          style={{ top: `${hour * GRID_PIXELS_PER_HOUR}px`, height: `${GRID_PIXELS_PER_HOUR}px` }}
          title={`Schedule something at ${hour}:00`}
          aria-label={`Schedule an airing at ${hour}:00 on ${day.toLocaleDateString()}`}
          onClick={() => {
            const at = startOfDay(day);
            at.setHours(hour);
            onEmptyHour(at);
          }}
        />
      ))}
      {visible.map((programme) => (
        <AiringBlock
          key={`${programme.scheduleEntryId}-${dayKey(day)}`}
          programme={programme}
          day={day}
          conflicted={overlapping.has(programme.scheduleEntryId)}
          onClick={() => onPick(programme)}
        />
      ))}
    </div>
  );
}

interface AiringBlockProps {
  programme: Programme;
  day: Date;
  conflicted: boolean;
  onClick: () => void;
}

/** AiringBlock is one slot, clipped to the day column it is drawn in. */
function AiringBlock({ programme, day, conflicted, onClick }: AiringBlockProps) {
  // A programme running past midnight is drawn on both days, clipped to each.
  const startMinutes = Math.max(0, minutesIntoDay(programme.airsAt, day));
  const endMinutes = Math.min(MINUTES_PER_DAY, minutesIntoDay(programme.endsAt, day));
  const height = Math.max(endMinutes - startMinutes, 8);

  return (
    <button
      type="button"
      onClick={onClick}
      title={`${programme.title} · ${formatTime(programme.airsAt)}–${formatTime(programme.endsAt)}`}
      className={`absolute left-0.5 right-0.5 overflow-hidden rounded px-1 py-0.5 text-left text-[10px] leading-tight ${
        conflicted
          ? "bg-destructive/80 text-destructive-foreground ring-1 ring-destructive"
          : "bg-primary/80 text-primary-foreground hover:bg-primary"
      }`}
      style={{
        top: `${(startMinutes / 60) * GRID_PIXELS_PER_HOUR}px`,
        height: `${(height / 60) * GRID_PIXELS_PER_HOUR}px`,
      }}
    >
      <span className="block truncate font-medium">{programme.title}</span>
      <span className="block truncate opacity-80">{formatTime(programme.airsAt)}</span>
    </button>
  );
}

/**
 * overlaps returns the IDs of airings whose slots intersect another's.
 *
 * Programmes arrive ordered by air time, so a single sweep keeping the furthest
 * end seen so far is enough.
 */
function overlaps(programmes: Programme[]): Set<string> {
  const clashing = new Set<string>();
  let previous: Programme | null = null;
  for (const programme of programmes) {
    if (previous && new Date(programme.airsAt) < new Date(previous.endsAt)) {
      clashing.add(programme.scheduleEntryId);
      clashing.add(previous.scheduleEntryId);
    }
    if (!previous || new Date(programme.endsAt) > new Date(previous.endsAt)) {
      previous = programme;
    }
  }
  return clashing;
}

interface AiringDialogProps {
  programme: Programme | null;
  createAt: Date | null;
  onClose: () => void;
  onSaved: () => void;
  onRemove: (programme: Programme) => Promise<void>;
}

/** AiringDialog places a new airing or retimes an existing one. */
function AiringDialog({ programme, createAt, onClose, onSaved, onRemove }: AiringDialogProps) {
  const media = useMediaItems();
  const [error, setError] = useState<string | null>(null);
  const open = programme !== null || createAt !== null;

  const submit = async (form: FormData) => {
    const airsAt = fromLocalInput(String(form.get("airsAt") ?? ""));
    if (!airsAt) {
      setError("An air time is required.");
      return;
    }
    const body: Record<string, unknown> = {
      airsAt,
      joinInProgress: form.get("joinInProgress") === "on",
    };
    const mediaItemId = String(form.get("mediaItemId") ?? "");
    if (mediaItemId) body.mediaItemId = mediaItemId;
    const block = String(form.get("block") ?? "");
    if (block) body.block = block;

    setError(null);
    try {
      if (programme) {
        await mutate("PATCH", `/schedule-entries/${programme.scheduleEntryId}`, body);
      } else {
        if (!mediaItemId) {
          setError("Choose a media item to air.");
          return;
        }
        await mutate("POST", "/schedule-entries", body);
      }
      onSaved();
    } catch (err) {
      setError((err as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{programme ? "Edit airing" : "Schedule an airing"}</DialogTitle>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            void submit(new FormData(e.currentTarget));
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="airing-media">Media item</Label>
            <select
              id="airing-media"
              name="mediaItemId"
              defaultValue={programme?.mediaItemId ?? ""}
              className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm"
            >
              <option value="">—</option>
              {media.choices.map((choice) => (
                <option key={choice.value} value={choice.value}>
                  {choice.label}
                </option>
              ))}
            </select>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="airing-airs-at">Airs at</Label>
            <Input
              id="airing-airs-at"
              name="airsAt"
              type="datetime-local"
              required
              defaultValue={toLocalInput(programme?.airsAt ?? createAt?.toISOString())}
            />
            {programme && (
              <p className="text-xs text-muted-foreground">
                Runs {formatDuration(programme.runtimeSeconds)} until {formatTime(programme.endsAt)}.
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="airing-block">Block</Label>
            <select
              id="airing-block"
              name="block"
              defaultValue={programme?.block ?? ""}
              className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm"
            >
              <option value="">Derive from the air time</option>
              {SCHEDULE_BLOCKS.map((block) => (
                <option key={block} value={block}>
                  {block}
                </option>
              ))}
            </select>
          </div>

          <div className="flex items-center gap-2">
            <input
              id="airing-join"
              name="joinInProgress"
              type="checkbox"
              defaultChecked={programme?.joinInProgress ?? true}
              className="h-4 w-4"
            />
            <Label htmlFor="airing-join">Devices may join in progress</Label>
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}

          <DialogFooter className="gap-2">
            {programme && (
              <Button
                type="button"
                variant="destructive"
                onClick={() => {
                  void onRemove(programme).then(onClose);
                }}
              >
                <Trash2 /> Remove
              </Button>
            )}
            <Button type="submit">{programme ? "Save" : "Schedule"}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

interface CopyWeekDialogProps {
  open: boolean;
  weekStart: Date;
  onClose: () => void;
  onCopied: () => void;
}

/**
 * CopyWeekDialog re-airs the visible week later on.
 *
 * The server does the shifting, in whole calendar days and in the channel's own
 * zone, so a 9am programme is still 9am after the clocks change. It reports
 * what it could not place rather than silently dropping it.
 */
function CopyWeekDialog({ open, weekStart, onClose, onCopied }: CopyWeekDialogProps) {
  const [result, setResult] = useState<CopyWeekResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async (form: FormData) => {
    setBusy(true);
    setError(null);
    setResult(null);
    try {
      const body = {
        fromWeekStart: dayKey(weekStart),
        toWeekStart: String(form.get("toWeekStart") ?? ""),
        replace: form.get("replace") === "on",
      };
      setResult((await mutate("POST", "/schedule-entries/copy-week", body)) as CopyWeekResponse);
      onCopied();
    } catch (err) {
      setError((err as Error).message);
    }
    setBusy(false);
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          setResult(null);
          setError(null);
          onClose();
        }
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Copy week of {weekStart.toLocaleDateString()}</DialogTitle>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            void submit(new FormData(e.currentTarget));
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="copy-to">Copy to the week starting</Label>
            <Input
              id="copy-to"
              name="toWeekStart"
              type="date"
              required
              defaultValue={dayKey(addDays(weekStart, DAYS_PER_WEEK))}
            />
            <p className="text-xs text-muted-foreground">
              Each airing keeps its weekday and time of day.
            </p>
          </div>

          <div className="flex items-center gap-2">
            <input id="copy-replace" name="replace" type="checkbox" className="h-4 w-4" />
            <Label htmlFor="copy-replace">Clear the destination week first</Label>
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}
          {result && <CopyOutcome result={result} />}

          <DialogFooter>
            <Button type="submit" disabled={busy}>
              {busy ? "Copying…" : "Copy"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** CopyOutcome reports what the copy managed and what it refused. */
function CopyOutcome({ result }: { result: CopyWeekResponse }) {
  const skipped = result.skipped ?? [];
  return (
    <div className="space-y-2 rounded-md border p-3 text-sm">
      <div className="flex flex-wrap gap-2">
        <Badge variant="secondary">{result.copied} copied</Badge>
        {result.deleted > 0 && <Badge variant="outline">{result.deleted} replaced</Badge>}
        {skipped.length > 0 && <Badge variant="destructive">{skipped.length} skipped</Badge>}
      </div>
      {skipped.length > 0 && (
        <ul className="space-y-1 text-xs text-muted-foreground">
          {skipped.map((entry) => (
            <li key={`${entry.mediaItemId}-${entry.airsAt}`}>
              <span className="font-medium">{entry.title}</span> at {formatTime(entry.airsAt)} —{" "}
              {entry.reason}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
