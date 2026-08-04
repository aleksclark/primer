import { useMemo, useState } from "react";
import { ArrowDown, ArrowUp, ChevronLeft, ChevronRight, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { mutate } from "@/api/client";
import { useList } from "@/hooks/use-list";
import { cn } from "@/lib/utils";

/** ColumnDef describes one column of a resource table. */
export interface ColumnDef<T> {
  key: string;
  header: string;
  sortable?: boolean;
  render?: (row: T) => React.ReactNode;
}

/** FieldDef describes one input in the create/edit form. */
export interface FieldDef {
  key: string;
  label: string;
  type?: "text" | "number" | "select" | "checkbox" | "date" | "tags";
  options?: string[];
  required?: boolean;
  createOnly?: boolean;
}

interface ResourcePageProps<T extends { id: string }> {
  title: string;
  path: string;
  columns: ColumnDef<T>[];
  fields: FieldDef[];
  defaultSort?: string;
}

const PAGE_SIZE = 25;

const selectClassName = cn(
  "flex h-10 w-full rounded-none border border-input bg-surface-raised",
  "px-3.5 py-3 text-sm text-foreground",
  "focus-visible:border-primary focus-visible:outline focus-visible:outline-[length:var(--primer-focus-width)]",
  "focus-visible:outline-offset-[var(--primer-focus-offset)] focus-visible:outline-primary",
  "disabled:cursor-not-allowed disabled:text-rule-strong",
);

/**
 * ResourcePage is a complete admin CRUD screen: paginated + searchable
 * data table with create, edit, and delete dialogs. Every LMS resource
 * gets one of these with only column and field configuration.
 */
export function ResourcePage<T extends { id: string }>({
  title,
  path,
  columns,
  fields,
  defaultSort = "created_at",
}: ResourcePageProps<T>) {
  const [q, setQ] = useState("");
  const [offset, setOffset] = useState(0);
  const [sort, setSort] = useState(defaultSort);
  const [dir, setDir] = useState<"asc" | "desc">("desc");
  const [editing, setEditing] = useState<T | null>(null);
  const [creating, setCreating] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const query = useMemo(
    () => ({ limit: PAGE_SIZE, offset, q: q || undefined, sort, dir }),
    [offset, q, sort, dir],
  );
  const { data, loading, error, refresh } = useList<T>(path, query);

  const toggleSort = (key: string) => {
    if (sort === key) {
      setDir(dir === "asc" ? "desc" : "asc");
    } else {
      setSort(key);
      setDir("asc");
    }
    setOffset(0);
  };

  const submitForm = async (form: FormData, existing: T | null) => {
    const body: Record<string, unknown> = {};
    for (const field of fields) {
      if (existing && field.createOnly) continue;
      const raw = form.get(field.key);
      if (field.type === "checkbox") {
        body[field.key] = raw === "on";
        continue;
      }
      if (raw == null || raw === "") continue;
      if (field.type === "date") {
        // The API models calendar days as instants, so a date-only input is
        // sent as its UTC midnight rather than through the browser's zone.
        body[field.key] = `${String(raw)}T00:00:00Z`;
        continue;
      }
      if (field.type === "tags") {
        body[field.key] = String(raw)
          .split(",")
          .map((t) => t.trim())
          .filter(Boolean);
        continue;
      }
      body[field.key] = field.type === "number" ? Number(raw) : raw;
    }
    try {
      setFormError(null);
      if (existing) {
        await mutate("PATCH", `${path}/${existing.id}`, body);
      } else {
        await mutate("POST", path, body);
      }
      setEditing(null);
      setCreating(false);
      refresh();
    } catch (err) {
      setFormError((err as Error).message);
    }
  };

  const remove = async (row: T) => {
    if (!confirm(`Delete this ${title.replace(/s$/, "").toLowerCase()}?`)) return;
    await mutate("DELETE", `${path}/${row.id}`);
    refresh();
  };

  const total = data?.totalCount ?? 0;
  const pageStart = total === 0 ? 0 : offset + 1;
  const pageEnd = Math.min(offset + PAGE_SIZE, total);

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-4 border-b border-rule-strong pb-4">
        <div className="space-y-1">
          <p className="type-label text-muted-foreground">Resource</p>
          <h1 className="type-h1 text-foreground">{title}</h1>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Input
            placeholder="Search…"
            value={q}
            onChange={(e) => {
              setQ(e.target.value);
              setOffset(0);
            }}
            className="w-64"
          />
          <Button variant="outline" size="icon" onClick={refresh} title="Refresh">
            <RefreshCw />
          </Button>
          <Button
            onClick={() => {
              setFormError(null);
              setCreating(true);
            }}
          >
            <Plus /> New
          </Button>
        </div>
      </div>

      {error && (
        <p className="type-label border border-attention px-3 py-2 text-attention" role="alert">
          {error}
        </p>
      )}

      <div className="border border-border bg-background">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              {columns.map((col) => (
                <TableHead key={col.key}>
                  {col.sortable ? (
                    <button
                      type="button"
                      className="type-label inline-flex items-center gap-1 text-muted-foreground hover:text-foreground"
                      onClick={() => toggleSort(col.key)}
                    >
                      {col.header}
                      {sort === col.key &&
                        (dir === "asc" ? <ArrowUp className="h-3 w-3" /> : <ArrowDown className="h-3 w-3" />)}
                    </button>
                  ) : (
                    col.header
                  )}
                </TableHead>
              ))}
              <TableHead className="w-24" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && !data ? (
              <TableRow>
                <TableCell colSpan={columns.length + 1} className="h-24 text-center text-muted-foreground">
                  Loading…
                </TableCell>
              </TableRow>
            ) : data?.items.length ? (
              data.items.map((row) => (
                <TableRow key={row.id}>
                  {columns.map((col) => (
                    <TableCell key={col.key}>
                      {col.render
                        ? col.render(row)
                        : String((row as Record<string, unknown>)[col.key] ?? "")}
                    </TableCell>
                  ))}
                  <TableCell>
                    <div className="flex gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => {
                          setFormError(null);
                          setEditing(row);
                        }}
                        title="Edit"
                      >
                        <Pencil />
                      </Button>
                      <Button variant="ghost" size="icon" onClick={() => remove(row)} title="Delete">
                        <Trash2 />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={columns.length + 1} className="h-24 text-center text-muted-foreground">
                  No results.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>

      <div className="flex items-center justify-between gap-4">
        <span className="type-label text-muted-foreground">
          {pageStart}–{pageEnd} of {total}
        </span>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
          >
            <ChevronLeft /> Prev
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={offset + PAGE_SIZE >= total}
            onClick={() => setOffset(offset + PAGE_SIZE)}
          >
            Next <ChevronRight />
          </Button>
        </div>
      </div>

      <Dialog
        open={creating || editing !== null}
        onOpenChange={(open) => {
          if (!open) {
            setCreating(false);
            setEditing(null);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? `Edit ${title}` : `New ${title}`}</DialogTitle>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              void submitForm(new FormData(e.currentTarget), editing);
            }}
          >
            {fields
              .filter((f) => !(editing && f.createOnly))
              .map((field) => {
                const existing = editing
                  ? (editing as Record<string, unknown>)[field.key]
                  : undefined;
                return (
                  <div key={field.key} className="space-y-2">
                    <Label htmlFor={field.key}>{field.label}</Label>
                    {field.type === "select" ? (
                      <select
                        id={field.key}
                        name={field.key}
                        defaultValue={existing != null ? String(existing) : ""}
                        required={field.required && !editing}
                        className={selectClassName}
                      >
                        <option value="">—</option>
                        {field.options?.map((opt) => (
                          <option key={opt} value={opt}>
                            {opt}
                          </option>
                        ))}
                      </select>
                    ) : field.type === "checkbox" ? (
                      <input
                        id={field.key}
                        name={field.key}
                        type="checkbox"
                        defaultChecked={Boolean(existing)}
                        className="h-4 w-4 rounded-none border border-border accent-[var(--primer-color-accent)]"
                      />
                    ) : (
                      <Input
                        id={field.key}
                        name={field.key}
                        type={inputType(field.type)}
                        defaultValue={defaultInputValue(field, existing)}
                        required={field.required && !editing}
                        aria-invalid={formError ? true : undefined}
                      />
                    )}
                  </div>
                );
              })}
            {formError && (
              <p className="type-label text-attention" role="alert">
                {formError}
              </p>
            )}
            <DialogFooter>
              <Button type="submit">{editing ? "Save" : "Create"}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}

/** inputType maps a field type onto the HTML input type that edits it. */
function inputType(type: FieldDef["type"]): string {
  switch (type) {
    case "number":
      return "number";
    case "date":
      return "date";
    default:
      return "text";
  }
}

/**
 * defaultInputValue renders an existing value for its editor: dates are
 * trimmed to the day the input expects, tag lists to a comma-separated line.
 */
function defaultInputValue(field: FieldDef, value: unknown): string {
  if (value == null) return "";
  if (field.type === "date") return String(value).slice(0, 10);
  if (field.type === "tags") return Array.isArray(value) ? value.join(", ") : String(value);
  return String(value);
}
