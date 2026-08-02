import { useCallback, useEffect, useState } from "react";
import { Check, Download, RefreshCw, Search, TriangleAlert } from "lucide-react";
import type { components } from "@/api/schema";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { get, imageURL, mutate } from "@/api/client";
import {
  classCol,
  codeCol,
  directPlayCol,
  durationCol,
  tagsCol,
  textCol,
} from "@/lib/columns";
import { MEDIA_CLASSES } from "@/lib/constants";

type Schemas = components["schemas"];
type MediaItem = Schemas["MediaItem"];
type BrowseItem = Schemas["BrowseItem"];
type SyncResponse = Schemas["SyncResponse"];

/**
 * Codecs and containers the RK3318 target box can hardware-decode. Anything
 * else would force Jellyfin to transcode, which the NAS cannot sustain, so an
 * import is pre-flagged as not direct-playable and the parent sees it before a
 * student hits a stalled stream.
 */
const DIRECT_PLAY_VIDEO = ["h264", "hevc", "h265", "mpeg4", "vp8", "vp9"];
const DIRECT_PLAY_AUDIO = ["aac", "mp3", "ac3", "opus", "vorbis", "flac"];

/** directPlayIssues lists the reasons an item would need transcoding. */
function directPlayIssues(item: Pick<BrowseItem, "videoCodec" | "audioCodec">): string[] {
  const issues: string[] = [];
  const video = item.videoCodec?.toLowerCase() ?? "";
  const audio = item.audioCodec?.toLowerCase() ?? "";
  if (video && !DIRECT_PLAY_VIDEO.includes(video)) issues.push(`video codec ${item.videoCodec}`);
  if (audio && !DIRECT_PLAY_AUDIO.includes(audio)) issues.push(`audio codec ${item.audioCodec}`);
  return issues;
}

/** LibraryPage manages the curated media library imported from Jellyfin. */
export function LibraryPage() {
  const [browsing, setBrowsing] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [syncResult, setSyncResult] = useState<SyncResponse | null>(null);
  const [syncError, setSyncError] = useState<string | null>(null);
  const [importCount, setImportCount] = useState(0);

  const runSync = async () => {
    setSyncing(true);
    setSyncError(null);
    setSyncResult(null);
    try {
      setSyncResult((await mutate("POST", "/jellyfin/sync")) as SyncResponse);
    } catch (err) {
      setSyncError((err as Error).message);
    } finally {
      setSyncing(false);
    }
  };

  return (
    <div className="space-y-4">
      <ResourcePage<MediaItem>
        refreshToken={importCount}
        title="Library"
        path="/media-items"
        description="Curated Jellyfin items. Classification drives watch-once rationing and what gets reported to Primer as instructional time."
        defaultSort="title"
        defaultDir="asc"
        canCreate={false}
        toolbar={
          <>
            <Button variant="outline" onClick={() => setBrowsing(true)}>
              <Search /> Browse Jellyfin
            </Button>
            <Button variant="outline" onClick={() => void runSync()} disabled={syncing}>
              <RefreshCw /> {syncing ? "Syncing…" : "Sync metadata"}
            </Button>
          </>
        }
        columns={[
          {
            key: "artwork",
            header: "",
            render: (row) => (
              <img
                src={imageURL(row.id)}
                alt=""
                className="h-12 w-8 rounded object-cover bg-muted"
                onError={(e) => {
                  e.currentTarget.style.visibility = "hidden";
                }}
              />
            ),
          },
          {
            key: "title",
            header: "Title",
            sortable: true,
            render: (row) => (
              <div className="space-y-0.5">
                <div className="font-medium">{row.title}</div>
                {row.orphanedAt && (
                  <Badge variant="destructive">Missing from Jellyfin</Badge>
                )}
              </div>
            ),
          },
          classCol<MediaItem>(),
          directPlayCol<MediaItem>(),
          durationCol<MediaItem>("runtime_seconds", "Runtime", (r) => r.runtimeSeconds, {
            sortable: true,
          }),
          tagsCol<MediaItem>("subjectTags", "Subjects", (r) => r.subjectTags),
          tagsCol<MediaItem>("standardCodes", "Standards", (r) => r.standardCodes),
          codeCol<MediaItem>("codecs", "Codecs", (r) =>
            [r.container, r.videoCodec, r.audioCodec].filter(Boolean).join(" / "),
          ),
          textCol<MediaItem>("qualityNotes", "Quality notes", (r) => r.qualityNotes),
        ]}
        fields={[
          { key: "title", label: "Title" },
          {
            key: "class",
            label: "Class",
            type: "select",
            options: MEDIA_CLASSES,
            help: "Entertainment is rationed to one play per availability window. Educational and mixed are replayable and reported to Primer.",
          },
          { key: "subjectTags", label: "Subject tags", type: "tags", help: "e.g. science, physics" },
          {
            key: "standardCodes",
            label: "Standard codes",
            type: "tags",
            help: "Curriculum standards this item provides evidence for.",
          },
          {
            key: "directPlayOk",
            label: "Direct play OK",
            type: "checkbox",
            help: "Uncheck to withhold the item from devices: the TV box cannot transcode.",
          },
          { key: "qualityNotes", label: "Quality notes" },
          { key: "overview", label: "Overview" },
          { key: "runtimeSeconds", label: "Runtime (seconds)", type: "number" },
        ]}
      />

      {syncError && <p className="text-sm text-destructive">Sync failed: {syncError}</p>}
      {syncResult && (
        <div className="rounded-md border bg-muted/30 p-3 text-sm">
          Checked {syncResult.checked}, updated {syncResult.updated}
          {syncResult.orphaned && syncResult.orphaned.length > 0 && (
            <span className="text-destructive">
              {" "}
              — {syncResult.orphaned.length} item(s) no longer exist in Jellyfin
            </span>
          )}
        </div>
      )}

      <BrowseDialog
        open={browsing}
        onClose={() => setBrowsing(false)}
        onImported={() => setImportCount((n) => n + 1)}
      />
    </div>
  );
}

interface BrowseDialogProps {
  open: boolean;
  onClose: () => void;
  onImported: () => void;
}

/**
 * BrowseDialog lists the Jellyfin library and imports items into the curated
 * set. Import defaults the class to entertainment (the safe, rationed choice)
 * and carries over the cached metadata plus a direct-play verdict; the parent
 * then classifies properly from the library table.
 */
function BrowseDialog({ open, onClose, onImported }: BrowseDialogProps) {
  const [search, setSearch] = useState("");
  const [items, setItems] = useState<BrowseItem[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [importing, setImporting] = useState<string | null>(null);
  const [imported, setImported] = useState<Set<string>>(new Set());

  const load = useCallback(
    async (signal?: AbortSignal) => {
      setLoading(true);
      setError(null);
      try {
        const params = new URLSearchParams({ limit: "100" });
        if (search) params.set("q", search);
        const res = await get<Schemas["BrowseResponse"]>(`/jellyfin/browse?${params}`, signal);
        setItems(res.items ?? []);
      } catch (err) {
        if ((err as Error).name !== "AbortError") setError((err as Error).message);
      } finally {
        setLoading(false);
      }
    },
    [search],
  );

  useEffect(() => {
    if (!open) return;
    const controller = new AbortController();
    const timer = setTimeout(() => void load(controller.signal), 250);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [open, load]);

  const importItem = async (item: BrowseItem) => {
    setImporting(item.jellyfinItemId);
    setError(null);
    try {
      await mutate("POST", "/media-items", {
        jellyfinItemId: item.jellyfinItemId,
        title: item.title,
        sortTitle: item.sortTitle || undefined,
        overview: item.overview || undefined,
        class: "entertainment",
        runtimeSeconds: item.runtimeSeconds || undefined,
        container: item.container || undefined,
        videoCodec: item.videoCodec || undefined,
        audioCodec: item.audioCodec || undefined,
        imageTag: item.imageTag || undefined,
        directPlayOk: directPlayIssues(item).length === 0,
        qualityNotes: directPlayIssues(item).join("; ") || undefined,
      });
      setImported((prev) => new Set(prev).add(item.jellyfinItemId));
      onImported();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setImporting(null);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle>Browse Jellyfin</DialogTitle>
        </DialogHeader>

        <div className="space-y-1.5">
          <Label htmlFor="browse-search">Search</Label>
          <Input
            id="browse-search"
            placeholder="Filter by title…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>

        {error && <p className="text-sm text-destructive">{error}</p>}

        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Title</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Codecs</TableHead>
                <TableHead>Direct play</TableHead>
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading && !items ? (
                <TableRow>
                  <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                    Loading…
                  </TableCell>
                </TableRow>
              ) : items?.length ? (
                items.map((item) => {
                  const issues = directPlayIssues(item);
                  const done = item.imported || imported.has(item.jellyfinItemId);
                  return (
                    <TableRow key={item.jellyfinItemId}>
                      <TableCell className="font-medium">{item.title}</TableCell>
                      <TableCell className="text-muted-foreground">{item.type}</TableCell>
                      <TableCell>
                        <code className="text-xs">
                          {[item.container, item.videoCodec, item.audioCodec].filter(Boolean).join(" / ")}
                        </code>
                      </TableCell>
                      <TableCell>
                        {issues.length === 0 ? (
                          <Badge variant="secondary">OK</Badge>
                        ) : (
                          <span className="inline-flex items-center gap-1 text-xs text-destructive">
                            <TriangleAlert className="h-3 w-3" />
                            {issues.join("; ")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        {done ? (
                          <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                            <Check className="h-3 w-3" /> Imported
                          </span>
                        ) : (
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={importing === item.jellyfinItemId}
                            onClick={() => void importItem(item)}
                          >
                            <Download /> Import
                          </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  );
                })
              ) : (
                <TableRow>
                  <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                    No library items.
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Done
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
