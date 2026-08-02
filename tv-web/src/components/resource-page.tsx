import { useEffect, useMemo, useState } from "react";
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
import { fromLocalInput, toLocalInput } from "@/lib/datetime";

/** ColumnDef describes one column of a resource table. */
export interface ColumnDef<T> {
  key: string;
  header: string;
  sortable?: boolean;
  render?: (row: T) => React.ReactNode;
}

/** Choice is a labelled select option, for foreign keys whose value is a UUID. */
export interface Choice {
  value: string;
  label: string;
}

/** FieldDef describes one input in the create/edit form. */
export interface FieldDef {
  key: string;
  label: string;
  type?: "text" | "number" | "select" | "checkbox" | "datetime" | "tags";
  options?: string[];
  choices?: Choice[];
  required?: boolean;
  createOnly?: boolean;
  help?: string;
}

/** RowAction is an extra per-row button beside edit and delete. */
export interface RowAction<T> {
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  run: (row: T) => Promise<void> | void;
}

interface ResourcePageProps<T extends { id: string }> {
  title: string;
  path: string;
  columns: ColumnDef<T>[];
  fields: FieldDef[];
  defaultSort?: string;
  defaultDir?: "asc" | "desc";
  description?: string;
  filter?: string[];
  rowActions?: RowAction<T>[];
  toolbar?: React.ReactNode;
  canCreate?: boolean;
  /** canEdit is false for resources the server exposes no update endpoint for. */
  canEdit?: boolean;
  /** searchable is false for resources the server has no search columns for. */
  searchable?: boolean;
  /** refreshToken reloads the table when an out-of-band write changes it. */
  refreshToken?: number;
}

const PAGE_SIZE = 25;

/**
 * ResourcePage is a complete admin CRUD screen: paginated + searchable
 * data table with create, edit, and delete dialogs. Every TV resource
 * gets one of these with only column and field configuration.
 */
export function ResourcePage<T extends { id: string }>({
  title,
  path,
  columns,
  fields,
  defaultSort = "created_at",
  defaultDir = "desc",
  description,
  filter,
  rowActions,
  toolbar,
  canCreate = true,
  canEdit = true,
  searchable = true,
  refreshToken,
}: ResourcePageProps<T>) {
  const [q, setQ] = useState("");
  const [offset, setOffset] = useState(0);
  const [sort, setSort] = useState(defaultSort);
  const [dir, setDir] = useState<"asc" | "desc">(defaultDir);
  const [editing, setEditing] = useState<T | null>(null);
  const [creating, setCreating] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const query = useMemo(
    () => ({ limit: PAGE_SIZE, offset, q: q || undefined, sort, dir, filter }),
    [offset, q, sort, dir, JSON.stringify(filter)],
  );
  const { data, loading, error, refresh } = useList<T>(path, query);

  useEffect(() => {
    if (refreshToken != null) refresh();
  }, [refreshToken, refresh]);

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
      if (field.type === "datetime") {
        const iso = fromLocalInput(String(raw));
        if (iso) body[field.key] = iso;
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

  /**
   * runWrite performs an out-of-form write and always reloads afterwards. The
   * failure has to be shown: a delete or a revoke that silently does nothing
   * would otherwise leave the parent believing it worked.
   */
  const runWrite = async (work: () => Promise<void> | void) => {
    setActionError(null);
    try {
      await work();
    } catch (err) {
      setActionError((err as Error).message);
    }
    refresh();
  };

  const remove = (row: T) =>
    runWrite(async () => {
      if (!confirm(`Delete this ${title.replace(/s$/, "").toLowerCase()}?`)) return;
      await mutate("DELETE", `${path}/${row.id}`);
    });

  const runAction = (action: RowAction<T>, row: T) => runWrite(() => action.run(row));

  const total = data?.totalCount ?? 0;
  const pageStart = total === 0 ? 0 : offset + 1;
  const pageEnd = Math.min(offset + PAGE_SIZE, total);
  const actionCount = (rowActions?.length ?? 0) + (canEdit ? 2 : 1);

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
          {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
        </div>
        <div className="flex items-center gap-2">
          {toolbar}
          {searchable && (
            <Input
              placeholder="Search…"
              value={q}
              onChange={(e) => {
                setQ(e.target.value);
                setOffset(0);
              }}
              className="w-64"
            />
          )}
          <Button variant="outline" size="icon" onClick={refresh} title="Refresh">
            <RefreshCw />
          </Button>
          {canCreate && (
            <Button onClick={() => { setFormError(null); setCreating(true); }}>
              <Plus /> New
            </Button>
          )}
        </div>
      </div>

      {error && <p className="text-sm text-destructive">{error}</p>}
      {actionError && <p className="text-sm text-destructive">{actionError}</p>}

      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              {columns.map((col) => (
                <TableHead key={col.key}>
                  {col.sortable ? (
                    <button
                      className="inline-flex items-center gap-1 hover:text-foreground"
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
              <TableHead style={{ width: `${actionCount * 2.5}rem` }} />
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
                      {rowActions?.map((action) => (
                        <Button
                          key={action.label}
                          variant="ghost"
                          size="icon"
                          onClick={() => void runAction(action, row)}
                          title={action.label}
                        >
                          <action.icon />
                        </Button>
                      ))}
                      {canEdit && (
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => { setFormError(null); setEditing(row); }}
                          title="Edit"
                        >
                          <Pencil />
                        </Button>
                      )}
                      <Button variant="ghost" size="icon" onClick={() => void remove(row)} title="Delete">
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

      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span>
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
              .map((field) => (
                <ResourceField
                  key={field.key}
                  field={field}
                  editing={editing !== null}
                  value={editing ? (editing as Record<string, unknown>)[field.key] : undefined}
                />
              ))}
            {formError && <p className="text-sm text-destructive">{formError}</p>}
            <DialogFooter>
              <Button type="submit">{editing ? "Save" : "Create"}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}

interface ResourceFieldProps {
  field: FieldDef;
  editing: boolean;
  value: unknown;
}

/** ResourceField renders one create/edit input for its declared field type. */
function ResourceField({ field, editing, value }: ResourceFieldProps) {
  const required = field.required && !editing;

  const control = () => {
    switch (field.type) {
      case "select": {
        const choices = field.choices ?? field.options?.map((o) => ({ value: o, label: o })) ?? [];
        return (
          <select
            id={field.key}
            name={field.key}
            defaultValue={value != null ? String(value) : ""}
            required={required}
            className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm"
          >
            <option value="">—</option>
            {choices.map((choice) => (
              <option key={choice.value} value={choice.value}>
                {choice.label}
              </option>
            ))}
          </select>
        );
      }
      case "checkbox":
        return (
          <input
            id={field.key}
            name={field.key}
            type="checkbox"
            defaultChecked={Boolean(value)}
            className="h-4 w-4"
          />
        );
      case "datetime":
        return (
          <Input
            id={field.key}
            name={field.key}
            type="datetime-local"
            defaultValue={toLocalInput(value as string | null | undefined)}
            required={required}
          />
        );
      case "tags":
        return (
          <Input
            id={field.key}
            name={field.key}
            placeholder="comma,separated"
            defaultValue={Array.isArray(value) ? (value as string[]).join(", ") : ""}
          />
        );
      default:
        return (
          <Input
            id={field.key}
            name={field.key}
            type={field.type === "number" ? "number" : "text"}
            defaultValue={value != null ? String(value) : ""}
            required={required}
          />
        );
    }
  };

  return (
    <div className="space-y-1.5">
      <Label htmlFor={field.key}>{field.label}</Label>
      {control()}
      {field.help && <p className="text-xs text-muted-foreground">{field.help}</p>}
    </div>
  );
}
