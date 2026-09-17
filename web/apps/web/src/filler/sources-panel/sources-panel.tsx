import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FetchFillerSourceOutputBody } from "@loomarr/api/models/fetchFillerSourceOutputBody";
import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";
import type { FillerSourcePreviewItemDTO } from "@loomarr/api/models/fillerSourcePreviewItemDTO";
import type { FillerSourceSuggestionDTO } from "@loomarr/api/models/fillerSourceSuggestionDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { formatBytes, formatRelative, pluralize } from "@loomarr/core/format";
import { useQueries, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { toast } from "sonner";
import { useAuth } from "@/auth/use-auth";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { FillerSources } from "@/components/loomarr/filler/filler-sources";
import { SourceSearch } from "@/components/loomarr/filler/source-search";
import { LocationPicker } from "@/components/loomarr/settings/installation-location";
// ⚠ No `Card` and no `Label`. The local add flow lives directly inside Your files, and its fields
// are labelled by their per-kind PLACEHOLDER plus `aria-label` — a static visible label above an
// input whose meaning changes with the kind would contradict it.
import { Button } from "@/components/ui/button";
import { Caption } from "@/components/ui/caption";
import { Disclosure } from "@/components/ui/disclosure";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { ProviderSourceFinder } from "./provider-source-finder";
import { SourceContentPreview } from "./source-content-preview";
import { SourceItemPreviewDialog } from "./source-item-preview-dialog";
import type { SourcesPanelProps } from "./sources-panel.type";

// Per-kind copy for "Add a source" (the mock's `newSourcePlaceholder`).
//
// ⚠ One table rather than nested ternaries at the two call sites. With two kinds a ternary was
// readable; with four it is a bug waiting to happen — and the two fields must stay in step, since
// a label promising a path beside a placeholder showing a URL is worse than either alone.
//
// ⚠ The FOLDER copy says "full path" deliberately. The server refuses a relative path (it would
// resolve against Loomarr's working directory, which differs between a container and a `go run`),
// so saying so up front is cheaper than a 400 that arrives after the operator has typed.
const SOURCE_KIND_COPY = {
  folder: { label: "Folder path", placeholder: "/data/filler — a full path Loomarr can read" },
  library: { label: "Library name", placeholder: "Commercials — as it appears on your media server" },
} as const;

type QueuedClipJob = { sourceID: string; clipID: string; jobID: string };
type AutomaticDownloadMode = "defaults" | "custom" | "never";
type DownloadIntervalUnit = "minutes" | "hours" | "days";
type DownloadSchedulePreset = "6h" | "12h" | "daily" | "weekly" | "custom";
const QUEUED_CLIP_JOBS_KEY = "loomarr:source-item-jobs";

const readQueuedClipJobs = (): QueuedClipJob[] => {
  try {
    const parsed = JSON.parse(sessionStorage.getItem(QUEUED_CLIP_JOBS_KEY) ?? "[]");
    if (!Array.isArray(parsed)) return [];
    return parsed
      .filter(
        (item): item is QueuedClipJob =>
          typeof item?.sourceID === "string" &&
          typeof item?.clipID === "string" &&
          typeof item?.jobID === "string",
      )
      .slice(-50);
  } catch {
    return [];
  }
};

const rememberQueuedClipJobs = (jobs: QueuedClipJob[]): void => {
  try {
    sessionStorage.setItem(QUEUED_CLIP_JOBS_KEY, JSON.stringify(jobs.slice(-50)));
  } catch {
    // Browser storage is only correlation; the acquisition resource remains authoritative.
  }
};

const sourceKindLabel = (source: FillerSourceDTO) => {
  switch (source.kind) {
    case "archive":
      return "Archive.org collection";
    case "youtube":
      return "YouTube channel or playlist";
    case "library":
      return "Media server library";
    default:
      return "Folder on this server";
  }
};

const checkResultText = (source: FillerSourceDTO, result: FetchFillerSourceOutputBody) => {
  const limit = ` This check could add up to ${result.maxPerCheck} ${result.maxPerCheck === 1 ? "clip" : "clips"}.`;
  const skipped =
    result.skipped > 0
      ? ` ${result.skipped} ${result.skipped === 1 ? "clip was" : "clips were"} already known and skipped.`
      : "";
  if (result.stoppedBy === "catalog") {
    return `Couldn’t add clips from ${source.target} because the clip limit is full.${limit}`;
  }
  if (result.queued > 0) {
    return `${result.queued} new ${result.queued === 1 ? "clip was" : "clips were"} queued from ${source.target}.${limit}${skipped}`;
  }
  if (result.added > 0 || result.updated > 0) {
    return `Found ${result.added} new and ${result.updated} updated ${result.added + result.updated === 1 ? "clip" : "clips"} in ${source.target}.`;
  }
  return `No new clips were found in ${source.target}.${limit}${skipped}`;
};

const readinessLabel = (source: FillerSourceDTO) => {
  switch (source.readiness) {
    case "off":
    case "provider_off":
      return "Paused";
    case "needs_location":
      return "Location needed";
    case "out_of_area":
      return "Not used in your area";
    case "unavailable":
      return "Couldn’t check this source";
    case "not_configured":
      return "Setup needed";
    default:
      return "Ready";
  }
};

const registeredSourceURL = (source: FillerSourceDTO) => {
  const uri = source.uri?.trim();
  if (!uri) return;
  if (source.kind === "youtube") return uri;
  if (source.kind !== "archive") return;
  if (/^https?:\/\//i.test(uri)) return uri;
  return `https://archive.org/details/${encodeURIComponent(uri)}`;
};

const downloadIntervalParts = (seconds: number): { amount: number; unit: DownloadIntervalUnit } => {
  if (seconds > 0 && seconds % 86400 === 0) return { amount: seconds / 86400, unit: "days" };
  if (seconds >= 3600 && seconds % 3600 === 0) return { amount: seconds / 3600, unit: "hours" };
  return { amount: Math.max(seconds / 60, 1), unit: "minutes" };
};

const downloadIntervalSeconds = (amount: number, unit: DownloadIntervalUnit): number =>
  Math.round(Math.max(amount, 1) * ({ days: 86400, hours: 3600, minutes: 60 } as const)[unit]);

const downloadIntervalMaximum = (unit: DownloadIntervalUnit): number =>
  ({ days: 7, hours: 168, minutes: 10080 })[unit];

const downloadSchedulePreset = (seconds: number): DownloadSchedulePreset => {
  if (seconds === 6 * 3600) return "6h";
  if (seconds === 12 * 3600) return "12h";
  if (seconds === 24 * 3600) return "daily";
  if (seconds === 7 * 24 * 3600) return "weekly";
  return "custom";
};

const downloadScheduleSeconds: Record<Exclude<DownloadSchedulePreset, "custom">, number> = {
  "6h": 6 * 3600,
  "12h": 12 * 3600,
  daily: 24 * 3600,
  weekly: 7 * 24 * 3600,
};

// SourcesPanel — the Sources tab of the filler page (§10 V35/V37/V38c): registered sources
// (folders, libraries, archive.org, YouTube), each switchable, fetchable and (if `removable`)
// forgettable, plus per-source search for archive.org and the "Add a source" form.
const SourcesPanel = ({ sources, sourcesError }: SourcesPanelProps) => {
  const queryClient = useQueryClient();
  const { isAdmin } = useAuth();
  const readinessQuery = fillerApi.useFillerReadiness();
  const readiness = unwrap(readinessQuery.data, (body) => body);
  const storage = readiness?.storage;
  const cleanupPreviewQuery = fillerApi.usePreviewFillerStorageCleanup({
    query: { enabled: isAdmin && storage?.pausedBy === "host_reserve" },
  });
  const cleanupPreview = unwrap(cleanupPreviewQuery.data, (body) => body);
  const cleanupStorage = fillerApi.useCleanupFillerStorage({
    mutation: {
      onSuccess: async (response) => {
        const result = unwrap(response, (body) => body);
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: fillerApi.getFillerReadinessQueryKey() }),
          queryClient.invalidateQueries({ queryKey: fillerApi.getPreviewFillerStorageCleanupQueryKey() }),
        ]);
        if (!result) return;
        if (result.failedItems > 0) {
          toast.error("Some temporary files could not be removed", {
            description: `${pluralize(result.failedItems, "folder")} stayed in place. Loomarr will try them again later.`,
          });
          return;
        }
        toast.success(
          result.removedItems > 0 ? `Freed ${formatBytes(result.removedBytes)}` : "Nothing needed cleaning",
          { description: "Your filler library was not changed." },
        );
      },
    },
  });
  const cleanupResult = unwrap(cleanupStorage.data, (body) => body);

  const [selectedSourceID, setSelectedSourceID] = useState<string>();
  const [sourcePreview, setSourcePreview] = useState<{
    sourceID: string;
    result: FillerSourceSuggestionDTO;
  }>();
  const [checkResult, setCheckResult] = useState<FetchFillerSourceOutputBody>();
  const [browseOpen, setBrowseOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [downloadMode, setDownloadMode] = useState<AutomaticDownloadMode>("defaults");
  const [downloadEverySeconds, setDownloadEverySeconds] = useState(6 * 3600);
  const [downloadMaxPerCheck, setDownloadMaxPerCheck] = useState(10);
  const [editingCustomDownloadSchedule, setEditingCustomDownloadSchedule] = useState(false);
  const [downloadSaveError, setDownloadSaveError] = useState<string>();
  const downloadSaveButton = useRef<HTMLButtonElement>(null);
  const [searchPreview, setSearchPreview] = useState<FillerSourcePreviewItemDTO>();
  const sourceTrigger = useRef<HTMLElement>(null);
  const selectedSource = sources.find((source) => source.id === selectedSourceID);

  const fetchSource = fillerApi.useFetchFillerSource({
    mutation: {
      onSuccess: (response) => {
        setCheckResult(unwrap(response, (body) => body));
        // Both: the catalog changed AND the per-source counts derive from it.
        void queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerQueryKey() });
        void queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerSourcesQueryKey() });
      },
    },
  });

  // ⚠ `useSyncFiller` and `useTagFiller` were called here and are gone (V38c). Both were
  // whole-catalog buttons in the header; the sync runs on its own schedule and drains the watch
  // folder every pass, and tagging follows it. The per-source "Fetch now" on the Sources tab
  // uses `fetchSource` below, which is a different route — so nothing here is left unreachable.

  // Switching a source on or off (V35). ⚠ The switch withdraws a source from future scanning,
  // searching and downloading; clips already in the catalog are untouched, which is why nothing
  // here invalidates the clip list.
  const [togglingSource, setTogglingSource] = useState<string>();
  const toggleSource = fillerApi.useSetFillerSourceEnabled({
    mutation: {
      onSettled: () => setTogglingSource(undefined),
      onSuccess: () =>
        void queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerSourcesQueryKey() }),
    },
  });

  // Forgetting a registered source (V37). ⚠ Like the switch, this does NOT touch the clip list —
  // deleting a source forgets the registration, never the files, which are real and may already
  // be tagged and pinned into a channel. Nothing here invalidates the catalog, deliberately.
  // "Add a source" (V37). The kind is explicit rather than sniffed from the URI — see the form.
  // ⚠ All FOUR kinds, and the union must list them — it still said `"archive" | "youtube"` after
  // V38c made folders and libraries addable, so the dropdown offered two options the state could
  // not hold. It defaults to `library` because that is the mock's first option, and the first
  // option is what an operator sees before touching anything.
  const [newSourceKind, setNewSourceKind] = useState<"folder" | "library">("library");
  const [newSourceURI, setNewSourceURI] = useState("");
  const addSource = fillerApi.useAddFillerSource({
    mutation: {
      onSuccess: () => {
        setNewSourceURI("");
        toast.success("Source added", { description: "Loomarr will check it on its download schedule." });
        void queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerSourcesQueryKey() });
      },
    },
  });

  const [togglingProvider, setTogglingProvider] = useState<"archive" | "youtube">();
  const toggleProvider = fillerApi.useSetFillerProviderEnabled({
    mutation: {
      onSettled: () => setTogglingProvider(undefined),
      // Keep the provider in its pending folded state until the refreshed read model arrives.
      // Clearing `togglingProvider` before this promise settles would briefly reopen enabled-looking
      // children between the PATCH response and the GET that reports `providerEnabled: false`.
      onSuccess: () => queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerSourcesQueryKey() }),
    },
  });

  // Source-specific geography is an exception, not part of adding every source. Empty values
  // mean follow the Installation geography; an explicit value is tucked under Source settings for a
  // collection whose real coverage differs.
  const [sourceCountry, setSourceCountry] = useState("");
  const [sourceMarket, setSourceMarket] = useState("");
  const resolveSourcePreview = fillerApi.useResolveFillerSource();
  const openSource = (source: (typeof sources)[number]) => {
    sourceTrigger.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setSelectedSourceID(source.id);
    setCheckResult(undefined);
    setSourceCountry(source.country ?? "");
    setSourceMarket(source.market ?? "");
    setSourceQuery("");
    setSubmittedQuery("");
    setStatIds([]);
    setSourcePreview(undefined);
    setSearchPreview(undefined);
    setBrowseOpen(false);
    setSettingsOpen(false);
    setDownloadMode(source.automaticDownloads?.mode ?? "defaults");
    setDownloadEverySeconds(source.automaticDownloads?.everySeconds || 6 * 3600);
    setDownloadMaxPerCheck(source.automaticDownloads?.maxPerCheck ?? 10);
    setEditingCustomDownloadSchedule(
      downloadSchedulePreset(source.automaticDownloads?.everySeconds || 6 * 3600) === "custom",
    );
    setDownloadSaveError(undefined);
    const uri = source.uri?.trim();
    if ((source.kind === "archive" || source.kind === "youtube") && source.providerEnabled && uri) {
      resolveSourcePreview.mutate(
        { kind: source.kind, data: { input: uri } },
        {
          onSuccess: (response) => {
            const result = unwrap(response, (body) => body);
            if (result) setSourcePreview({ sourceID: source.id, result });
          },
        },
      );
    }
  };

  const [removingSource, setRemovingSource] = useState<string>();
  const removeSource = fillerApi.useDeleteFillerSource({
    mutation: {
      onSettled: () => setRemovingSource(undefined),
      onSuccess: () => {
        setSelectedSourceID(undefined);
        toast.success("Source removed", {
          description: "Clips it already brought in stay in your catalog.",
        });
        void queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerSourcesQueryKey() });
      },
    },
  });

  // Searching a source for clips to add (V35). ⚠ `submitted` is a SEPARATE state from what the
  // operator is typing: the query runs on submit, not on keystroke. Each search costs archive.org
  // one Solr request plus a metadata call per row, so firing per character would be both slow and
  // rude — and the results would flicker under the cursor.
  const [sourceQuery, setSourceQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");
  const discover = fillerApi.useDiscoverFiller(
    { q: submittedQuery, collection: selectedSource?.uri },
    // Admin-only on the server, and only once something has actually been submitted — an
    // enabled query with an empty q would 422 on mount.
    {
      query: {
        enabled:
          isAdmin &&
          submittedQuery.trim().length >= 2 &&
          selectedSource?.kind === "archive" &&
          Boolean(selectedSource.uri),
      },
    },
  );

  // Runtime + quality for the rows on screen (V35).
  //
  // ⚠ These are NOT part of the search, and the measurement is the reason: each id is one
  // `/metadata/<id>` call against archive.org, and a full page of 25 measured 22.6s. So the
  // rows paint immediately and this fills in for what the operator is actually looking at.
  //
  // ⚠ `statIds` only ever GROWS within a result set, and never re-asks for an id it already
  // holds — the observer fires repeatedly as rows scroll, and re-requesting would spend real
  // upstream calls on answers already in hand.
  const [statIds, setStatIds] = useState<string[]>([]);
  const statsQuery = fillerApi.useDiscoverFillerStats(
    { id: statIds },
    { query: { enabled: isAdmin && statIds.length > 0 } },
  );
  const discoveredStats = unwrap(statsQuery.data, (b) => b.stats) ?? {};

  // Keep only the browser's item→job correlation here; status itself is always read from the
  // durable acquisition resource. This lets a dropped SSE frame cost latency rather than truth.
  const [queuedJobs, setQueuedJobs] = useState<QueuedClipJob[]>(readQueuedClipJobs);
  const [queueingId, setQueueingId] = useState<string>();
  const selectedQueuedJobs = queuedJobs.filter((job) => job.sourceID === selectedSourceID);
  const acquisitionQueries = useQueries({
    queries: selectedQueuedJobs.map((job) => {
      const cached = queryClient.getQueryData<fillerApi.GetFillerAcquisitionQueryResult>(
        fillerApi.getGetFillerAcquisitionQueryKey(job.jobID),
      );
      const run = unwrap(cached, (body) => body);
      return {
        ...fillerApi.getGetFillerAcquisitionQueryOptions(job.jobID),
        refetchInterval: run && (run.status === "success" || run.status === "error") ? false : 1_500,
      };
    }),
  });
  const queueStatus = Object.fromEntries(
    selectedQueuedJobs
      .map((job, index) => ({ job, query: acquisitionQueries[index] }))
      .map(({ job, query }) => {
        const run = unwrap(query?.data, (body) => body);
        return [job.clipID, query?.isError ? "error" : (run?.status ?? "queued")];
      }),
  ) as Record<string, "queued" | "running" | "success" | "error">;
  const queueClip = fillerApi.useQueueFillerSourceItem({
    mutation: {
      onSettled: () => setQueueingId(undefined),
      onSuccess: (response, variables) => {
        const result = unwrap(response, (body) => body);
        if (result?.jobId) {
          setQueuedJobs((previous) => {
            const next = [
              ...previous.filter(
                (job) => job.sourceID !== variables.id || job.clipID !== variables.data.remoteId,
              ),
              { sourceID: variables.id, clipID: variables.data.remoteId, jobID: result.jobId },
            ].slice(-50);
            rememberQueuedClipJobs(next);
            return next;
          });
        }
      },
    },
  });

  const discoveredResults = unwrap(discover.data, (b) => b.items) ?? [];
  const localSourceSetup = isAdmin ? (
    <div className="flex flex-col gap-3">
      <p className="font-medium text-sm">Add a folder or library</p>
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(event) => {
          event.preventDefault();
          if (!newSourceURI.trim()) return;
          addSource.mutate({
            data: {
              kind: newSourceKind,
              uri: newSourceURI.trim(),
            },
          });
        }}
      >
        <Select
          value={newSourceKind}
          onValueChange={(value) => {
            setNewSourceKind(value as typeof newSourceKind);
          }}
        >
          <SelectTrigger id="new-source-kind" className="w-45" aria-label="Kind of source to add">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="library">Media server library</SelectItem>
            <SelectItem value="folder">Watched folder</SelectItem>
          </SelectContent>
        </Select>
        <Input
          id="new-source-uri"
          className="min-w-64 flex-1 font-mono"
          value={newSourceURI}
          placeholder={SOURCE_KIND_COPY[newSourceKind].placeholder}
          aria-label={SOURCE_KIND_COPY[newSourceKind].label}
          onChange={(event) => setNewSourceURI(event.target.value)}
        />
        <Button type="submit" variant="outline" disabled={addSource.isPending || !newSourceURI.trim()}>
          {addSource.isPending ? "Adding…" : "+ Add source"}
        </Button>
      </form>
      {addSource.error != null && <ErrorState error={addSource.error} />}
      <Caption>Uses your location. Every clip is checked before it can play.</Caption>
    </div>
  ) : undefined;

  return (
    <div className="flex flex-col gap-6">
      {storage ? (
        <section aria-labelledby="filler-storage-heading" className="rounded-lg border border-border bg-card p-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0 flex-1">
              <h2 id="filler-storage-heading" className="font-medium text-sm">
                Storage
              </h2>
              <p className="mt-1 text-sm">
                {formatBytes(storage.managedBytes)} used for filler · {formatBytes(storage.freeBytes)} free on
                this drive
              </p>
              <p className="mt-1 text-muted-foreground text-xs">
                {storage.pausedBy === "host_reserve"
                  ? `New filler is paused so this drive keeps ${formatBytes(storage.hardReserveBytes)} free.`
                  : storage.pausedBy === "library_limit"
                    ? `New filler is paused at its ${formatBytes(storage.softBudgetBytes)} allowance.`
                    : storage.pausedBy === "capacity_unavailable" || storage.pausedBy === "estimate_unknown"
                      ? "Loomarr cannot safely check the available space in this folder."
                      : `Automatic downloads will pause before this drive has less than ${formatBytes(storage.hardReserveBytes)} free.`}
                {storage.reservedBytes > 0
                  ? ` ${formatBytes(storage.reservedBytes)} is set aside for work in progress.`
                  : ""}
              </p>
            </div>
            <Button
              variant="ghost"
              size="sm"
              render={
                <a
                  href={`/filler/settings/${
                    storage.pausedBy === "capacity_unavailable" || storage.pausedBy === "estimate_unknown"
                      ? "folders"
                      : "storage"
                  }`}
                />
              }
            >
              {storage.pausedBy === "capacity_unavailable" || storage.pausedBy === "estimate_unknown"
                ? "Choose folder"
                : storage.pausedBy === "library_limit"
                  ? "Change allowance"
                  : "Storage options"}
            </Button>
          </div>

          {storage.pausedBy === "host_reserve" ? (
            <div className="mt-3 border-border border-t pt-3">
              {cleanupPreviewQuery.isLoading ? (
                <p aria-live="polite" className="text-muted-foreground text-sm">
                  Checking for old temporary downloads…
                </p>
              ) : cleanupPreview && cleanupPreview.items > 0 ? (
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <p className="text-muted-foreground text-sm">
                    {formatBytes(cleanupPreview.bytes)} in {pluralize(cleanupPreview.items, "unfinished download")} is
                    safe to remove. Clips in your library stay untouched.
                  </p>
                  <Button
                    type="button"
                    size="sm"
                    disabled={cleanupStorage.isPending}
                    onClick={() => cleanupStorage.mutate()}
                  >
                    {cleanupStorage.isPending
                      ? "Freeing space…"
                      : `Free ${formatBytes(cleanupPreview.bytes)}`}
                  </Button>
                </div>
              ) : (
                <p className="text-muted-foreground text-sm">
                  There are no old temporary downloads Loomarr can safely remove. Free space elsewhere on this
                  drive or remove filler clips you no longer want.
                </p>
              )}
              {cleanupResult && cleanupResult.removedItems > 0 ? (
                <p role="status" className="mt-2 text-muted-foreground text-xs">
                  Removed {pluralize(cleanupResult.removedItems, "temporary folder")} · freed{" "}
                  {formatBytes(cleanupResult.removedBytes)}.
                </p>
              ) : null}
              {cleanupPreviewQuery.error || cleanupStorage.error ? (
                <p role="alert" className="mt-2 text-destructive text-sm">
                  Temporary files could not be checked or removed. Nothing in your library was changed.
                </p>
              ) : null}
            </div>
          ) : null}
        </section>
      ) : readinessQuery.error ? (
        <ErrorState error={readinessQuery.error} onRetry={() => readinessQuery.refetch()} />
      ) : null}

      <FillerSources
        sources={sources}
        onSelect={openSource}
        selectedId={selectedSourceID}
        onToggleEnabled={(id, enabled) => {
          setTogglingSource(id);
          toggleSource.mutate({ id, data: { enabled } });
        }}
        toggling={toggleSource.isPending ? togglingSource : null}
        onToggleProvider={(kind, enabled) => {
          setTogglingProvider(kind);
          toggleProvider.mutate({ kind, data: { enabled } });
        }}
        togglingProvider={toggleProvider.isPending ? togglingProvider : null}
        renderLocalSetup={localSourceSetup}
        error={
          toggleSource.error?.detail ??
          fetchSource.error?.detail ??
          removeSource.error?.detail ??
          toggleProvider.error?.detail ??
          sourcesError ??
          null
        }
        renderProviderSetup={(provider) => {
          if (provider.kind !== "archive" && provider.kind !== "youtube") return undefined;
          return <ProviderSourceFinder kind={provider.kind} enabled={provider.enabled} />;
        }}
      />

      <Sheet
        open={Boolean(selectedSource)}
        onOpenChange={(open) => {
          if (!open) {
            setSelectedSourceID(undefined);
            setSourcePreview(undefined);
            setSearchPreview(undefined);
            setBrowseOpen(false);
            setSettingsOpen(false);
          }
        }}
        swipeDirection="right"
      >
        {selectedSource && (
          <SheetContent finalFocus={sourceTrigger}>
            <SheetHeader>
              <SheetTitle>
                {selectedSource.id === "folder" ? "Drop folder" : selectedSource.target}
              </SheetTitle>
              <SheetDescription>{sourceKindLabel(selectedSource)}</SheetDescription>
            </SheetHeader>

            <div className="flex flex-col gap-6 p-6">
              <section aria-labelledby="source-status-heading" className="flex flex-col gap-3">
                <div>
                  <h3 id="source-status-heading" className="font-medium">
                    {readinessLabel(selectedSource)}
                  </h3>
                  <p className="mt-1 text-muted-foreground text-sm">{selectedSource.detail}</p>
                  <p className="mt-1 text-muted-foreground text-xs">
                    {selectedSource.incoming > 0
                      ? `${selectedSource.count} ready · `
                      : `${selectedSource.count} ${selectedSource.count === 1 ? "clip" : "clips"}`}
                    {selectedSource.incoming > 0 && (
                      <a
                        href="/filler/incoming"
                        className="underline decoration-border underline-offset-2 hover:text-foreground"
                      >
                        {`${selectedSource.incoming} being checked`}
                      </a>
                    )}
                    {selectedSource.lastCheckedAt
                      ? ` · last checked ${formatRelative(selectedSource.lastCheckedAt)}`
                      : " · not checked yet"}
                  </p>
                  {selectedSource.kind === "folder" && (
                    <p className="mt-3 break-all font-mono text-muted-foreground text-xs">
                      {selectedSource.uri ?? selectedSource.target}
                    </p>
                  )}
                </div>

                {selectedSource.actions.includes("fetch") && (
                  <Button
                    type="button"
                    className="self-start"
                    disabled={fetchSource.isPending}
                    onClick={() => {
                      setCheckResult(undefined);
                      fetchSource.mutate({ params: { id: selectedSource.id } });
                    }}
                  >
                    {fetchSource.isPending ? `Looking in ${selectedSource.target}…` : "Look for new clips"}
                  </Button>
                )}

                {checkResult && checkResult.sourceId === selectedSource.id && (
                  <p role="status" className="rounded-md bg-muted/40 px-3 py-2 text-sm">
                    {checkResultText(selectedSource, checkResult)}
                  </p>
                )}
                {fetchSource.error && (
                  <p role="alert" className="rounded-md bg-destructive/10 px-3 py-2 text-destructive text-sm">
                    {fetchSource.error.detail}
                  </p>
                )}
              </section>

              {registeredSourceURL(selectedSource) &&
                (selectedSource.kind === "archive" || selectedSource.kind === "youtube") && (
                  <section aria-labelledby="source-preview-heading" className="border-border border-t pt-5">
                    <h3 id="source-preview-heading" className="font-medium">
                      From this source
                    </h3>
                    <div className="mt-2">
                      <SourceContentPreview
                        kind={selectedSource.kind}
                        canonicalUrl={
                          sourcePreview?.sourceID === selectedSource.id
                            ? sourcePreview.result.canonicalUrl
                            : registeredSourceURL(selectedSource)!
                        }
                        previewItems={
                          sourcePreview?.sourceID === selectedSource.id
                            ? sourcePreview.result.previewItems
                            : undefined
                        }
                        loading={selectedSource.providerEnabled && resolveSourcePreview.isPending}
                        error={
                          !selectedSource.providerEnabled
                            ? `Resume ${selectedSource.kind === "archive" ? "Archive.org" : "YouTube"} to refresh these examples.`
                            : resolveSourcePreview.error
                              ? "Examples aren’t available right now. You can still open the source."
                              : null
                        }
                      />
                    </div>
                  </section>
                )}

              {selectedSource.actions.includes("search") && (
                <Disclosure
                  open={browseOpen}
                  onOpenChange={setBrowseOpen}
                  className="border-border border-t pt-5"
                >
                  <Disclosure.SectionTrigger
                    label={`${browseOpen ? "Hide" : "Show"} tools for finding a specific clip`}
                    title="Find a specific clip"
                    description="Search this Archive.org collection by title or keyword."
                  />
                  <Disclosure.Panel className="pt-4">
                    <SourceSearch
                      results={discoveredResults}
                      total={unwrap(discover.data, (body) => body.total) ?? undefined}
                      stats={discoveredStats}
                      onVisible={(visible) =>
                        setStatIds((previous) => {
                          const next = visible.filter((id) => !previous.includes(id));
                          return next.length > 0 ? [...previous, ...next] : previous;
                        })
                      }
                      loadingStats={statsQuery.isFetching ? statIds : []}
                      query={sourceQuery}
                      onQueryChange={setSourceQuery}
                      onSearch={() => {
                        setSubmittedQuery(sourceQuery);
                        setStatIds([]);
                      }}
                      onPreview={(clip) =>
                        setSearchPreview({
                          title: clip.title || clip.id,
                          url: clip.url,
                          durationMs: discoveredStats[clip.id]?.durationMs ?? clip.durationMs,
                        })
                      }
                      onQueue={(clip) => {
                        if (!selectedSource) return;
                        setQueueingId(clip.id);
                        queueClip.mutate({
                          id: selectedSource.id,
                          data: { remoteId: clip.id, url: clip.url },
                        });
                      }}
                      queueStatus={queueStatus}
                      queueing={queueClip.isPending ? queueingId : null}
                      searching={discover.isFetching}
                      error={discover.error?.detail ?? queueClip.error?.detail ?? null}
                    />
                  </Disclosure.Panel>
                </Disclosure>
              )}

              {(selectedSource.automaticDownloads ||
                selectedSource.actions.includes("edit_location") ||
                selectedSource.actions.includes("remove")) && (
                <Disclosure
                  open={settingsOpen}
                  onOpenChange={setSettingsOpen}
                  className="border-border border-t pt-5"
                >
                  <Disclosure.SectionTrigger
                    label={`${settingsOpen ? "Hide" : "Show"} source settings`}
                    title="Source settings"
                    description="Automatic downloads, location, and removal"
                  />
                  <Disclosure.Panel className="pt-4">
                    <div className="flex flex-col gap-3">
                      {selectedSource.automaticDownloads && (
                        <div className="flex flex-col gap-3">
                          <div>
                            <p className="font-medium text-sm">Automatic downloads</p>
                            <p className="mt-1 text-muted-foreground text-sm">
                              {selectedSource.automaticDownloads.summary}
                            </p>
                            {selectedSource.automaticDownloads.nextCheckAt && (
                              <p className="mt-1 text-muted-foreground text-xs">
                                Next automatic check{" "}
                                {formatRelative(selectedSource.automaticDownloads.nextCheckAt)}
                              </p>
                            )}
                          </div>
                          <Select
                            value={downloadMode}
                            onValueChange={(value) => {
                              setDownloadMode(value as AutomaticDownloadMode);
                              if (value === "custom") {
                                setEditingCustomDownloadSchedule(
                                  downloadSchedulePreset(downloadEverySeconds) === "custom",
                                );
                              }
                            }}
                          >
                            <SelectTrigger aria-label="Automatic downloads for this source">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="defaults">Use automatic-download defaults</SelectItem>
                              <SelectItem value="custom">Use a different schedule</SelectItem>
                              <SelectItem value="never">Never download automatically</SelectItem>
                            </SelectContent>
                          </Select>
                          {downloadMode === "custom" && (
                            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                              <div className="flex flex-col gap-1.5">
                                <span className="text-sm">Look for new clips</span>
                                <Select
                                  value={
                                    editingCustomDownloadSchedule
                                      ? "custom"
                                      : downloadSchedulePreset(downloadEverySeconds)
                                  }
                                  onValueChange={(value) => {
                                    const preset = value as DownloadSchedulePreset;
                                    setEditingCustomDownloadSchedule(preset === "custom");
                                    if (preset !== "custom") {
                                      setDownloadEverySeconds(downloadScheduleSeconds[preset]);
                                    }
                                  }}
                                >
                                  <SelectTrigger aria-label="Source check schedule">
                                    <SelectValue />
                                  </SelectTrigger>
                                  <SelectContent>
                                    <SelectItem value="6h">Every 6 hours</SelectItem>
                                    <SelectItem value="12h">Every 12 hours</SelectItem>
                                    <SelectItem value="daily">Daily</SelectItem>
                                    <SelectItem value="weekly">Weekly</SelectItem>
                                    <SelectItem value="custom">Custom</SelectItem>
                                  </SelectContent>
                                </Select>
                              </div>
                              <div className="flex flex-col gap-1.5">
                                <span className="text-sm">Add up to</span>
                                <Input
                                  type="number"
                                  min={1}
                                  max={1000}
                                  aria-label="Clips per source check"
                                  value={downloadMaxPerCheck}
                                  onChange={(event) =>
                                    setDownloadMaxPerCheck(
                                      Math.min(1000, Math.max(Number(event.target.value), 1)),
                                    )
                                  }
                                />
                              </div>
                              {editingCustomDownloadSchedule && (
                                <div className="flex flex-col gap-1.5 sm:col-span-2">
                                  <span className="text-sm">Custom schedule</span>
                                  <div className="flex gap-2">
                                    <Input
                                      type="number"
                                      min={1}
                                      max={downloadIntervalMaximum(
                                        downloadIntervalParts(downloadEverySeconds).unit,
                                      )}
                                      step="any"
                                      aria-label="Source check interval"
                                      value={downloadIntervalParts(downloadEverySeconds).amount}
                                      onChange={(event) => {
                                        const current = downloadIntervalParts(downloadEverySeconds);
                                        const maximum = downloadIntervalMaximum(current.unit);
                                        setDownloadEverySeconds(
                                          downloadIntervalSeconds(
                                            Math.min(Number(event.target.value), maximum),
                                            current.unit,
                                          ),
                                        );
                                      }}
                                    />
                                    <Select
                                      value={downloadIntervalParts(downloadEverySeconds).unit}
                                      onValueChange={(value) => {
                                        const current = downloadIntervalParts(downloadEverySeconds);
                                        setDownloadEverySeconds(
                                          downloadIntervalSeconds(
                                            Math.min(
                                              current.amount,
                                              downloadIntervalMaximum(value as DownloadIntervalUnit),
                                            ),
                                            value as DownloadIntervalUnit,
                                          ),
                                        );
                                      }}
                                    >
                                      <SelectTrigger
                                        aria-label="Source check interval unit"
                                        className="w-28 shrink-0"
                                      >
                                        <SelectValue />
                                      </SelectTrigger>
                                      <SelectContent>
                                        <SelectItem value="minutes">minutes</SelectItem>
                                        <SelectItem value="hours">hours</SelectItem>
                                        <SelectItem value="days">days</SelectItem>
                                      </SelectContent>
                                    </Select>
                                  </div>
                                </div>
                              )}
                            </div>
                          )}
                          <Button
                            ref={downloadSaveButton}
                            type="button"
                            variant="outline"
                            className="self-start"
                            disabled={toggleSource.isPending}
                            onClick={() => {
                              setDownloadSaveError(undefined);
                              setTogglingSource(selectedSource.id);
                              toggleSource.mutate(
                                {
                                  id: selectedSource.id,
                                  data: {
                                    enabled: selectedSource.enabled,
                                    automaticDownloads:
                                      downloadMode === "custom"
                                        ? {
                                            mode: "custom",
                                            everySeconds: downloadEverySeconds,
                                            maxPerCheck: downloadMaxPerCheck,
                                          }
                                        : { mode: downloadMode },
                                  },
                                },
                                {
                                  onSuccess: () => toast.success("Automatic downloads updated"),
                                  onError: (error) =>
                                    setDownloadSaveError(
                                      error.detail ?? "Automatic downloads couldn’t be saved. Try again.",
                                    ),
                                  onSettled: () =>
                                    requestAnimationFrame(() => downloadSaveButton.current?.focus()),
                                },
                              );
                            }}
                          >
                            {toggleSource.isPending && togglingSource === selectedSource.id
                              ? "Saving…"
                              : "Save automatic downloads"}
                          </Button>
                          {downloadSaveError && (
                            <p
                              role="alert"
                              className="rounded-md bg-destructive/10 px-3 py-2 text-destructive text-sm"
                            >
                              {downloadSaveError}
                            </p>
                          )}
                        </div>
                      )}

                      {selectedSource.actions.includes("edit_location") && (
                        <div className="flex flex-col gap-3 border-border border-t pt-3">
                          <p className="text-muted-foreground text-sm">
                            Change this only when the source covers a different area from your location.
                          </p>
                          <LocationPicker
                            value={
                              sourceCountry
                                ? { country: sourceCountry, market: sourceMarket }
                                : {
                                    country: selectedSource.effectiveCountry ?? "",
                                    market: selectedSource.effectiveMarket ?? "",
                                  }
                            }
                            onChange={(location) => {
                              setSourceCountry(location.country);
                              setSourceMarket(location.market ?? "");
                            }}
                          />
                          <div className="flex flex-wrap gap-2">
                            <Button
                              type="button"
                              variant="outline"
                              disabled={toggleSource.isPending || !sourceCountry.trim()}
                              onClick={() => {
                                setTogglingSource(selectedSource.id);
                                toggleSource.mutate({
                                  id: selectedSource.id,
                                  data: {
                                    enabled: selectedSource.enabled,
                                    geography: {
                                      country: sourceCountry.trim().toUpperCase(),
                                      market: sourceMarket.trim(),
                                    },
                                  },
                                });
                              }}
                            >
                              Save different area
                            </Button>
                            {(sourceCountry.trim() || selectedSource.locationSource === "source") && (
                              <Button
                                type="button"
                                variant="ghost"
                                disabled={toggleSource.isPending}
                                onClick={() => {
                                  if (selectedSource.locationSource !== "source") {
                                    setSourceCountry("");
                                    setSourceMarket("");
                                    return;
                                  }
                                  setTogglingSource(selectedSource.id);
                                  toggleSource.mutate(
                                    {
                                      id: selectedSource.id,
                                      data: {
                                        enabled: selectedSource.enabled,
                                        geography: { country: "", market: "" },
                                      },
                                    },
                                    {
                                      onSuccess: () => {
                                        setSourceCountry("");
                                        setSourceMarket("");
                                      },
                                    },
                                  );
                                }}
                              >
                                Use my location
                              </Button>
                            )}
                          </div>
                        </div>
                      )}

                      {selectedSource.actions.includes("remove") && (
                        <div className="border-border border-t pt-3">
                          <Button
                            type="button"
                            variant="ghost"
                            disabled={removeSource.isPending}
                            onClick={() => {
                              setRemovingSource(selectedSource.id);
                              removeSource.mutate({ id: selectedSource.id });
                            }}
                            aria-label={`Remove ${selectedSource.target}`}
                          >
                            {removeSource.isPending && removingSource === selectedSource.id
                              ? "Removing…"
                              : "Remove source"}
                          </Button>
                          <p className="mt-1 text-muted-foreground text-xs">
                            Clips already downloaded stay in your library.
                          </p>
                        </div>
                      )}
                    </div>
                  </Disclosure.Panel>
                </Disclosure>
              )}

              <SourceItemPreviewDialog
                item={searchPreview}
                kind="archive"
                onClose={() => setSearchPreview(undefined)}
              />
            </div>
          </SheetContent>
        )}
      </Sheet>
    </div>
  );
};

export { SourcesPanel };
