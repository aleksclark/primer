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
import { useList, mutate } from "@/hooks/use-list";

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
  type?: "text" | "number" | "select" | "checkbox";
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
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-4">
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        <div className="flex items-center gap-2">
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
          <Button onClick={() => { setFormError(null); setCreating(true); }}>
            <Plus /> New
          </Button>
        </div>
      </div>

      {error && <p className="text-sm text-destructive">{error}</p>}

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
              <TableHead className="w-20" />
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
                        onClick={() => { setFormError(null); setEditing(row); }}
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
              .map((field) => {
                const existing = editing
                  ? (editing as Record<string, unknown>)[field.key]
                  : undefined;
                return (
                  <div key={field.key} className="space-y-1.5">
                    <Label htmlFor={field.key}>{field.label}</Label>
                    {field.type === "select" ? (
                      <select
                        id={field.key}
                        name={field.key}
                        defaultValue={existing != null ? String(existing) : ""}
                        required={field.required && !editing}
                        className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm"
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
                        className="h-4 w-4"
                      />
                    ) : (
                      <Input
                        id={field.key}
                        name={field.key}
                        type={field.type === "number" ? "number" : "text"}
                        defaultValue={existing != null ? String(existing) : ""}
                        required={field.required && !editing}
                      />
                    )}
                  </div>
                );
              })}
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
