import { useMemo, useState } from "react";
import { CalendarClock, ChevronLeft, ChevronRight, Hourglass, Table2 } from "lucide-react";
import type { components } from "@/api/schema";
import { ResourcePage } from "@/components/resource-page";
import { RotationPanel } from "@/components/rotation-panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { mutate } from "@/api/client";
import { useList } from "@/hooks/use-list";
import { useMediaItems } from "@/hooks/use-media-items";
import { dateTimeCol, textCol } from "@/lib/columns";
import { addDays, dayKey, formatDateTime, startOfDay } from "@/lib/datetime";

type AvailabilityWindow = components["schemas"]["AvailabilityWindow"];

/** WINDOW_LIMIT is the server's maximum page size for a list endpoint. */
const WINDOW_LIMIT = 200;

/** DAYS_SHOWN is the length of the rotation calendar view. */
const DAYS_SHOWN = 14;

/** DEFAULT_WINDOW_DAYS is how long a rotated-in window runs by default. */
const DEFAULT_WINDOW_DAYS = 7;

/**
 * AvailabilityPage manages the on-demand rotation. A window is what makes an
 * item playable, and for entertainment it is also the unit the watch-once
 * ledger consumes, so rotation is the parent's main content lever.
 *
 * The calendar view answers "what is available when"; the table view is the
 * standard CRUD surface for precise edits.
 */
export function AvailabilityPage() {
  const [view, setView] = useState<"calendar" | "table">("calendar");
  const [rotations, setRotations] = useState(0);
  const media = useMediaItems();

  const toolbar = (
    <Button variant="outline" onClick={() => setView(view === "calendar" ? "table" : "calendar")}>
      {view === "calendar" ? <Table2 /> : <CalendarClock />}
      {view === "calendar" ? "Table" : "Calendar"}
    </Button>
  );

  if (view === "calendar") {
    return (
      <div className="space-y-4">
        <RotationPanel key={rotations} onRotated={() => setRotations((n) => n + 1)} />
        <RotationCalendar key={`calendar-${rotations}`} toolbar={toolbar} />
      </div>
    );
  }

  return (
    <ResourcePage<AvailabilityWindow>
      title="Availability"
      path="/availability-windows"
      description="On-demand rotation windows. An entertainment item is playable once per window; educational and mixed items are replayable while a window is open."
      defaultSort="starts_at"
      defaultDir="desc"
      toolbar={toolbar}
      columns={[
        {
          key: "mediaItemId",
          header: "Media item",
          render: (row) => <span className="font-medium">{media.title(row.mediaItemId)}</span>,
        },
        dateTimeCol<AvailabilityWindow>("starts_at", "Starts", (r) => r.startsAt, { sortable: true }),
        dateTimeCol<AvailabilityWindow>("ends_at", "Ends", (r) => r.endsAt, { sortable: true }),
        {
          key: "status",
          header: "Status",
          render: (row) => <WindowStatus window={row} />,
        },
        textCol<AvailabilityWindow>("note", "Note", (r) => r.note),
      ]}
      fields={[
        {
          key: "mediaItemId",
          label: "Media item",
          type: "select",
          choices: media.choices,
          required: true,
          createOnly: true,
        },
        { key: "startsAt", label: "Starts at", type: "datetime", help: "Defaults to now when left blank." },
        { key: "endsAt", label: "Ends at", type: "datetime", required: true },
        { key: "note", label: "Note" },
      ]}
    />
  );
}

/**
 * expiryPatch is the update that takes a window off the air now.
 *
 * The server enforces `ends_at > starts_at`, so a window that has not started
 * yet cannot simply have its end pulled back to now — that would be rejected.
 * Expiring such a window pulls its start back with it: "expire" means "make
 * this unavailable", and a zero-length window in the past is exactly that.
 */
function expiryPatch(win: AvailabilityWindow, now: Date): Record<string, string> {
  const endsAt = now.toISOString();
  if (new Date(win.startsAt).getTime() >= now.getTime()) {
    return { startsAt: new Date(now.getTime() - 1000).toISOString(), endsAt };
  }
  return { endsAt };
}

/** WindowStatus renders whether a window is open, upcoming, or expired. */
function WindowStatus({ window }: { window: AvailabilityWindow }) {
  const now = Date.now();
  const starts = new Date(window.startsAt).getTime();
  const ends = new Date(window.endsAt).getTime();
  if (ends <= now) return <Badge variant="outline">Expired</Badge>;
  if (starts > now) return <Badge variant="secondary">Upcoming</Badge>;
  return <Badge>Open</Badge>;
}

interface RotationCalendarProps {
  toolbar: React.ReactNode;
}

/**
 * RotationCalendar lays the windows out over a fortnight so the parent can see
 * coverage and gaps at a glance, and offers the two bulk operations rotation
 * actually needs: expire everything that is currently open, and rotate a batch
 * of items in for the coming week.
 */
function RotationCalendar({ toolbar }: RotationCalendarProps) {
  const [anchor, setAnchor] = useState(() => startOfDay(new Date()));
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const media = useMediaItems();
  // Newest first: the server has no date-range filter on windows, so only one
  // page is loaded. Sorting ascending would fill that page with long-expired
  // history and silently hide the current rotation as the ledger grows.
  const query = useMemo(
    () => ({ limit: WINDOW_LIMIT, sort: "starts_at", dir: "desc" as const }),
    [],
  );
  const { data, loading, refresh } = useList<AvailabilityWindow>("/availability-windows", query);

  // A window past the page limit is invisible here, so the bulk actions cannot
  // honestly claim to cover "all" of anything. Say so rather than mislead.
  const truncated = (data?.totalCount ?? 0) > (data?.items.length ?? 0);

  const days = useMemo(
    () => Array.from({ length: DAYS_SHOWN }, (_, i) => addDays(anchor, i)),
    [anchor],
  );

  const rangeStart = anchor.getTime();
  const rangeEnd = addDays(anchor, DAYS_SHOWN).getTime();

  const visible = useMemo(
    () =>
      (data?.items ?? []).filter((w) => {
        const starts = new Date(w.startsAt).getTime();
        const ends = new Date(w.endsAt).getTime();
        return ends > rangeStart && starts < rangeEnd;
      }),
    [data, rangeStart, rangeEnd],
  );

  const openNow = useMemo(() => {
    const now = Date.now();
    return (data?.items ?? []).filter(
      (w) => new Date(w.startsAt).getTime() <= now && new Date(w.endsAt).getTime() > now,
    );
  }, [data]);

  // Selection survives navigating the date range, so count only the rows that
  // are on screen: those are the ones the bulk actions can actually act on.
  const selectedVisible = useMemo(
    () => visible.filter((w) => selected.has(w.id)),
    [visible, selected],
  );

  const toggle = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  /** expireNow ends the selected windows immediately, pulling items off-air. */
  const expireSelected = async () => {
    const windows = selectedVisible;
    if (windows.length === 0) return;
    if (!confirm(`Expire ${windows.length} window(s) now? Items become unavailable immediately.`)) return;
    const now = new Date();
    await runBulk(
      windows.map((w) => mutate("PATCH", `/availability-windows/${w.id}`, expiryPatch(w, now))),
    );
    setSelected(new Set());
  };

  /**
   * rotateSelected expires the selected windows and opens a fresh week for the
   * same items: the actual "expire & rotate" move at the end of a rotation.
   */
  const rotateSelected = async () => {
    const windows = selectedVisible;
    if (windows.length === 0) return;
    if (!confirm(`Expire ${windows.length} window(s) and reopen each item for ${DEFAULT_WINDOW_DAYS} days?`)) return;

    const now = new Date();
    const endsAt = addDays(now, DEFAULT_WINDOW_DAYS);
    await runBulk([
      ...windows.map((w) => mutate("PATCH", `/availability-windows/${w.id}`, expiryPatch(w, now))),
      ...windows.map((w) =>
        mutate("POST", "/availability-windows", {
          mediaItemId: w.mediaItemId,
          startsAt: now.toISOString(),
          endsAt: endsAt.toISOString(),
          note: w.note || undefined,
        }),
      ),
    ]);
    setSelected(new Set());
  };

  /** expireAllOpen is the panic button: everything currently on-air comes off. */
  const expireAllOpen = async () => {
    if (openNow.length === 0) return;
    const caveat = truncated ? " Only the windows loaded on this page are affected." : "";
    if (!confirm(`Expire all ${openNow.length} open window(s) now?${caveat}`)) return;
    const now = new Date();
    await runBulk(
      openNow.map((w) => mutate("PATCH", `/availability-windows/${w.id}`, expiryPatch(w, now))),
    );
    setSelected(new Set());
  };

  /**
   * runBulk awaits a batch of writes and always reloads afterwards. allSettled
   * rather than all: a bulk action is not a transaction, so some writes can
   * land while others fail, and the table has to be refreshed either way or it
   * shows state that is no longer true.
   */
  const runBulk = async (work: Promise<unknown>[]) => {
    setBusy(true);
    setError(null);
    const results = await Promise.allSettled(work);
    const failures = results.filter((r) => r.status === "rejected");
    if (failures.length > 0) {
      const [first] = failures;
      const detail = (first as PromiseRejectedResult).reason as Error;
      setError(
        failures.length === 1
          ? detail.message
          : `${failures.length} of ${results.length} changes failed: ${detail.message}`,
      );
    }
    setBusy(false);
    refresh();
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4 border-b border-rule-strong pb-4">
        <div className="space-y-1">
          <p className="type-label text-muted-foreground">Content</p>
          <h1 className="type-h1 text-foreground">Availability</h1>
          <p className="text-sm text-muted-foreground">
            On-demand rotation over {DAYS_SHOWN} days. Select windows to expire or rotate them.
          </p>
        </div>
        <div className="flex items-center gap-2">{toolbar}</div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Button variant="outline" size="sm" onClick={() => setAnchor(addDays(anchor, -DAYS_SHOWN))}>
          <ChevronLeft /> Earlier
        </Button>
        <Button variant="outline" size="sm" onClick={() => setAnchor(startOfDay(new Date()))}>
          Today
        </Button>
        <Button variant="outline" size="sm" onClick={() => setAnchor(addDays(anchor, DAYS_SHOWN))}>
          Later <ChevronRight />
        </Button>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          <span className="type-label text-muted-foreground">{selectedVisible.length} selected</span>
          <Button
            variant="outline"
            size="sm"
            disabled={busy || selectedVisible.length === 0}
            onClick={() => void expireSelected()}
          >
            <Hourglass /> Expire
          </Button>
          <Button
            size="sm"
            disabled={busy || selectedVisible.length === 0}
            onClick={() => void rotateSelected()}
          >
            <CalendarClock /> Expire &amp; rotate {DEFAULT_WINDOW_DAYS}d
          </Button>
          <Button
            variant="destructive"
            size="sm"
            disabled={busy || openNow.length === 0}
            onClick={() => void expireAllOpen()}
          >
            Expire all open ({openNow.length})
          </Button>
        </div>
      </div>

      {error && (
        <p className="type-label border border-attention px-3 py-2 text-attention" role="alert">
          {error}
        </p>
      )}

      <div className="overflow-x-auto border border-border">
        <div className="min-w-[56rem]">
          <div
            className="type-label grid border-b border-border bg-surface-raised text-muted-foreground"
            style={{ gridTemplateColumns: `16rem repeat(${DAYS_SHOWN}, minmax(0, 1fr))` }}
          >
            <div className="p-2">Media item</div>
            {days.map((day) => (
              <div key={dayKey(day)} className="border-l border-border p-2 text-center">
                <div>{day.toLocaleDateString(undefined, { weekday: "narrow" })}</div>
                <div className="text-muted-foreground">{day.getDate()}</div>
              </div>
            ))}
          </div>

          {loading && !data ? (
            <p className="p-6 text-center text-sm text-muted-foreground">Loading…</p>
          ) : visible.length === 0 ? (
            <p className="p-6 text-center text-sm text-muted-foreground">
              No windows in this range. Switch to the table view to add one.
            </p>
          ) : (
            visible.map((w) => (
              <WindowRow
                key={w.id}
                window={w}
                days={days}
                title={media.title(w.mediaItemId)}
                selected={selected.has(w.id)}
                onToggle={() => toggle(w.id)}
              />
            ))
          )}
        </div>
      </div>
    </div>
  );
}

interface WindowRowProps {
  window: AvailabilityWindow;
  days: Date[];
  title: string;
  selected: boolean;
  onToggle: () => void;
}

/** WindowRow draws one window's span across the calendar's day columns. */
function WindowRow({ window: win, days, title, selected, onToggle }: WindowRowProps) {
  const starts = new Date(win.startsAt).getTime();
  const ends = new Date(win.endsAt).getTime();
  const expired = ends <= Date.now();

  return (
    <div
      className="grid items-center border-b border-border text-sm last:border-b-0 hover:bg-surface-raised"
      style={{ gridTemplateColumns: `16rem repeat(${days.length}, minmax(0, 1fr))` }}
    >
      <label className="flex items-center gap-2 p-2">
        <input
          type="checkbox"
          checked={selected}
          onChange={onToggle}
          aria-label={`Select window for ${title}`}
          className="h-4 w-4 shrink-0 rounded-none border border-border accent-[var(--primer-color-accent)]"
        />
        <span className="truncate" title={`${title} · ${formatDateTime(win.startsAt)} → ${formatDateTime(win.endsAt)}`}>
          <span className={expired ? "text-muted-foreground line-through" : "font-medium"}>{title}</span>
        </span>
      </label>
      {days.map((day) => {
        const dayStart = day.getTime();
        const dayEnd = addDays(day, 1).getTime();
        const covered = ends > dayStart && starts < dayEnd;
        return (
          <div key={dayKey(day)} className="h-full border-l border-border p-1">
            {covered && (
              <div
                className={
                  expired
                    ? "h-5 rounded-none bg-border"
                    : "h-5 rounded-none bg-primary"
                }
                title={`${formatDateTime(win.startsAt)} → ${formatDateTime(win.endsAt)}`}
              />
            )}
          </div>
        );
      })}
    </div>
  );
}
