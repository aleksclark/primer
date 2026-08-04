import { Badge } from "@/components/ui/badge";
import type { ColumnDef } from "@/components/resource-page";
import { formatDateTime, formatDuration } from "@/lib/datetime";

/** Truncate a UUID for compact table display. */
export const shortId = (id?: string | null) => (id ? id.slice(0, 8) : "");

/** Format an ISO date string for table display. */
export const shortDate = (v?: string | null) => (v ? new Date(v).toLocaleDateString() : "");

/** Short UUID chip column (uses entity `id`). Sort key is the DB column name. */
export function idCol<T extends { id: string }>(): ColumnDef<T> {
  return {
    key: "id",
    header: "ID",
    render: (row) => (
      <code className="font-mono text-xs text-muted-foreground">{shortId(row.id)}</code>
    ),
  };
}

/** Created-at column with consistent sort key. */
export function createdCol<T extends { createdAt: string }>(): ColumnDef<T> {
  return {
    key: "created_at",
    header: "Created",
    sortable: true,
    render: (row) => shortDate(row.createdAt),
  };
}

/** Plain text column; `sortKey` defaults to `key` and should be the DB column name when sortable. */
export function textCol<T>(
  key: string,
  header: string,
  get: (row: T) => unknown,
  opts?: { sortable?: boolean; sortKey?: string },
): ColumnDef<T> {
  return {
    key: opts?.sortKey ?? key,
    header,
    sortable: opts?.sortable,
    render: (row) => {
      const v = get(row);
      return v == null ? "" : String(v);
    },
  };
}

/** Monospace code column. */
export function codeCol<T>(
  key: string,
  header: string,
  get: (row: T) => string | null | undefined,
  opts?: { sortable?: boolean; sortKey?: string },
): ColumnDef<T> {
  return {
    key: opts?.sortKey ?? key,
    header,
    sortable: opts?.sortable,
    render: (row) => <code className="font-mono text-xs">{get(row) ?? ""}</code>,
  };
}

/** Badge column for enum-like string fields. */
export function badgeCol<T>(
  key: string,
  header: string,
  get: (row: T) => string | null | undefined,
  opts?: { sortable?: boolean; sortKey?: string; variant?: "secondary" | "outline" | "default" | "destructive" },
): ColumnDef<T> {
  return {
    key: opts?.sortKey ?? key,
    header,
    sortable: opts?.sortable,
    render: (row) => {
      const v = get(row);
      return v ? <Badge variant={opts?.variant ?? "secondary"}>{v}</Badge> : "";
    },
  };
}

/** Short foreign-key UUID column (display only). */
export function shortIdCol<T>(key: string, header: string, get: (row: T) => string | null | undefined): ColumnDef<T> {
  return {
    key,
    header,
    render: (row) => shortId(get(row)),
  };
}

/** Date column; `sortKey` should be the DB column name when sortable. */
export function dateCol<T>(
  key: string,
  header: string,
  get: (row: T) => string | null | undefined,
  opts?: { sortable?: boolean; sortKey?: string },
): ColumnDef<T> {
  return {
    key: opts?.sortKey ?? key,
    header,
    sortable: opts?.sortable,
    render: (row) => shortDate(get(row)),
  };
}

/** Date-and-time column, for the instants that drive rotation and the grid. */
export function dateTimeCol<T>(
  key: string,
  header: string,
  get: (row: T) => string | null | undefined,
  opts?: { sortable?: boolean; sortKey?: string },
): ColumnDef<T> {
  return {
    key: opts?.sortKey ?? key,
    header,
    sortable: opts?.sortable,
    render: (row) => formatDateTime(get(row)),
  };
}

/** Runtime column rendering seconds as a human duration. */
export function durationCol<T>(
  key: string,
  header: string,
  get: (row: T) => number | null | undefined,
  opts?: { sortable?: boolean; sortKey?: string },
): ColumnDef<T> {
  return {
    key: opts?.sortKey ?? key,
    header,
    sortable: opts?.sortable,
    render: (row) => formatDuration(get(row)),
  };
}

/** Boolean column rendered as a yes/no badge, coloured for the "bad" case. */
export function boolCol<T>(
  key: string,
  header: string,
  get: (row: T) => boolean | null | undefined,
  opts?: { sortable?: boolean; sortKey?: string; trueLabel?: string; falseLabel?: string },
): ColumnDef<T> {
  return {
    key: opts?.sortKey ?? key,
    header,
    sortable: opts?.sortable,
    render: (row) =>
      get(row) ? (
        <Badge variant="secondary">{opts?.trueLabel ?? "Yes"}</Badge>
      ) : (
        <Badge variant="destructive">{opts?.falseLabel ?? "No"}</Badge>
      ),
  };
}

/** String-array column rendered as a row of chips. */
export function tagsCol<T>(
  key: string,
  header: string,
  get: (row: T) => string[] | null | undefined,
): ColumnDef<T> {
  return {
    key,
    header,
    render: (row) => {
      const tags = get(row) ?? [];
      if (tags.length === 0) return "";
      return (
        <div className="flex flex-wrap gap-1">
          {tags.map((tag) => (
            <Badge key={tag} variant="outline">
              {tag}
            </Badge>
          ))}
        </div>
      );
    },
  };
}

/** Media item class column; entertainment is the rationed one, so it stands out. */
export function classCol<T extends { class: string }>(): ColumnDef<T> {
  return {
    key: "class",
    header: "Class",
    sortable: true,
    render: (row) => (
      <Badge variant={row.class === "entertainment" ? "default" : "secondary"}>{row.class}</Badge>
    ),
  };
}

/**
 * Direct-play column. Direct play is a hard requirement on the target TV box
 * (transcoding is disabled), so a false value is a blocking defect and is
 * rendered as one.
 */
export function directPlayCol<T extends { directPlayOk: boolean }>(): ColumnDef<T> {
  return boolCol<T>("direct_play_ok", "Direct play", (r) => r.directPlayOk, {
    sortable: false,
    trueLabel: "OK",
    falseLabel: "Blocked",
  });
}
