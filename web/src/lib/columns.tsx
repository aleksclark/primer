import { Badge } from "@/components/ui/badge";
import type { ColumnDef } from "@/components/resource-page";

/** Truncate a UUID for compact table display. */
export const shortId = (id?: string | null) => (id ? id.slice(0, 8) : "");

/** Format an ISO date string for table display. */
export const shortDate = (v?: string | null) => (v ? new Date(v).toLocaleDateString() : "");

/** Short UUID chip column (uses entity `id`). Sort key is the DB column name. */
export function idCol<T extends { id: string }>(): ColumnDef<T> {
  return {
    key: "id",
    header: "ID",
    render: (row) => <code className="text-xs text-muted-foreground">{shortId(row.id)}</code>,
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
    render: (row) => <code>{get(row) ?? ""}</code>,
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

/** Grade level column with standard sort key. */
export function gradeCol<T extends { gradeLevel?: number | null }>(): ColumnDef<T> {
  return textCol<T>("grade_level", "Grade", (r) => r.gradeLevel ?? "", { sortable: true });
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

/**
 * Duration column rendering seconds as hours and minutes. Instructional time
 * is read in hours, so raw seconds would make the parent do the arithmetic.
 */
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
    render: (row) => {
      const seconds = get(row);
      if (seconds == null) return "";
      const hours = Math.floor(seconds / 3600);
      const minutes = Math.round((seconds % 3600) / 60);
      return hours > 0 ? `${hours}h ${minutes}m` : `${minutes}m`;
    },
  };
}
