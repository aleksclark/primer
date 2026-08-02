/**
 * Conversions between the API's RFC 3339 timestamps and the local-time values
 * that `datetime-local` inputs produce. Availability windows and schedule
 * entries are authored in the parent's local time but stored as instants, so
 * every form field has to round-trip through here.
 */

/** toLocalInput renders an ISO timestamp for a `datetime-local` input. */
export function toLocalInput(iso?: string | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** fromLocalInput converts a `datetime-local` value to an ISO timestamp. */
export function fromLocalInput(value: string): string | null {
  if (!value) return null;
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? null : d.toISOString();
}

/** startOfDay returns midnight local time on the given date. */
export function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

/** addDays returns a new date shifted by the given number of days. */
export function addDays(d: Date, days: number): Date {
  const out = new Date(d);
  out.setDate(out.getDate() + days);
  return out;
}

/** dayKey is the stable YYYY-MM-DD key used to bucket windows by local day. */
export function dayKey(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** formatDateTime renders an instant as a compact local date and time. */
export function formatDateTime(iso?: string | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  return Number.isNaN(d.getTime())
    ? ""
    : d.toLocaleString(undefined, {
        month: "short",
        day: "numeric",
        hour: "numeric",
        minute: "2-digit",
      });
}

/** startOfWeek returns the Monday on or before the given date, at midnight. */
export function startOfWeek(d: Date): Date {
  const start = startOfDay(d);
  // getDay() is Sunday-based; the programmed week runs Monday to Sunday
  // because a school week does.
  return addDays(start, -((start.getDay() + 6) % 7));
}

/** formatTime renders an instant as a bare local clock time. */
export function formatTime(iso?: string | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  return Number.isNaN(d.getTime())
    ? ""
    : d.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
}

/** minutesIntoDay is how far past the given day's local midnight an instant falls. */
export function minutesIntoDay(iso: string, day: Date): number {
  return (new Date(iso).getTime() - startOfDay(day).getTime()) / 60000;
}

/** formatDuration renders a runtime in seconds as "1h 42m". */
export function formatDuration(seconds?: number | null): string {
  if (!seconds || seconds <= 0) return "";
  // Round to whole minutes before splitting: rounding the sub-hour remainder
  // on its own renders 7199s as "1h 60m".
  const totalMinutes = Math.round(seconds / 60);
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return hours > 0 ? `${hours}h ${minutes}m` : `${minutes}m`;
}
