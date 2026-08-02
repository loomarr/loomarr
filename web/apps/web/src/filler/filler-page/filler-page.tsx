import type { ClipDTO } from "@loomarr/api";
import { fillerApi, isOk, settingsApi, toProblem, unwrap } from "@loomarr/api";
import { pluralize } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { RefreshCw, Sparkles } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { useAuth } from "@/auth";
import {
  ClipCard,
  CountTabs,
  EmptyState,
  ErrorState,
  FillerSources,
  IncomingPanel,
  PoolHealth,
} from "@/components/loomarr";
import {
  Button,
  Card,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui";
import { useLoomarrEventListener } from "@/events";
import { useDocumentTitle } from "@/lib";
import type { FillerSearch } from "@/routes/_authed/filler";
import { ClipTagDialog } from "../clip-tag-dialog";
import { IngestPanel } from "../ingest-panel";
import { PinClipDialog } from "../pin-clip-dialog";

// One cycled tag. Deliberately the same shape ClipCard's onCycle emits, so the card and the
// page cannot drift on what a retag carries.
type TagChange = Partial<Pick<ClipDTO, "era" | "audience" | "category">>;

// The bulk bar's three dropdowns (V35). ⚠ Each is INDEPENDENT — picking one sends only that
// field, and the server leaves the other two alone. A single "apply" that posted all three
// would blank whatever the operator had not touched, which is the failure the BE's per-field
// optionality exists to prevent.
//
// The vocabularies mirror the clip card's chips. ⚠ They deliberately omit the card's trailing
// "" / 0 entries: those exist there so CYCLING can pass through "unset", which is a different
// affordance from a menu — an "unset" item in a bulk menu is a one-click way to blank a
// hundred clips' tags, and nothing in this bar should be that easy.
// ⚠ Labelled "Set …", not "Era"/"Audience"/"Category". The catalog's FILTER bar already owns
// those names on this page, and two controls sharing an accessible name is a real ambiguity for
// anyone driving by keyboard or screen reader — a test caught it as "found multiple elements",
// which is the same collision seen from the outside. The verb also says what each one does:
// the filter narrows what you see, this changes what the clips ARE.
const BULK_TAG_FIELDS = [
  { key: "era", label: "Set era", options: ["1950", "1960", "1970", "1980", "1990", "2000", "2010", "2020"] },
  { key: "audience", label: "Set audience", options: ["kids", "family", "general", "late_night"] },
  {
    key: "category",
    label: "Set category",
    options: ["food", "toys", "auto", "retail", "media", "service"],
  },
] as const;

// FillerPage — the §10 clip catalog: browse, search, tag, sync, and (on the filler image)
// download. Filtering is client-driven but server-executed: the store already indexes
// these columns, so a query per filter change is cheaper and always correct, versus
// holding thousands of clips in memory to filter locally (§7.2 — no client index).
const FillerPage = () => {
  useDocumentTitle("Filler");
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { isAdmin } = useAuth();
  // Filters live in the URL (deep-linkable, shareable, back-button aware) — the route's
  // validateSearch narrows them. setFilters merges a partial change and writes with
  // `replace: true` so typing in the search box doesn't stack a history entry per keystroke.
  const {
    tab = "catalog",
    q = "",
    kind = "",
    audience = "",
    untagged = false,
  } = useSearch({ from: "/_authed/filler/" });
  const setFilters = (next: Partial<FillerSearch>) =>
    navigate({ to: "/filler", search: (prev) => ({ ...prev, ...next }), replace: true });
  const [tagging, setTagging] = useState<string>();
  const [pinning, setPinning] = useState<string>();

  const settings = settingsApi.useSettingsList();
  const features = unwrap(settings.data, (b) => b.features);
  const fillerConfigured = Boolean(features?.filler);

  const clips = fillerApi.useListFiller({
    ...(q ? { q } : {}),
    ...(kind ? { kind: kind as never } : {}),
    ...(audience ? { audience: audience as never } : {}),
    ...(untagged ? { untagged: true } : {}),
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerQueryKey() });

  // ⚠ The Discover query state that used to live here is GONE with its tab (V35). Searching a
  // source is moving onto the Sources tab, where it belongs — a search is something you do to a
  // source, not a destination of its own — and `GET /v1/filler/discover` is still the route
  // behind it. Deleting the state rather than leaving it wired to nothing: an unused query that
  // still fires is how a page keeps paying for a feature nobody can reach.

  // ⚠ Every hook stays ABOVE the `fillerConfigured` early return below. Placing these two
  // after it skipped them on the unconfigured path and crashed the page with "rendered more
  // hooks than during the previous render" — the whole catalog replaced by an error boundary.
  //
  // Sources are admin-only on the server, so a member's request would 403. Gating the QUERY
  // (not just the tab) keeps a member's console clean.
  const sourcesQuery = fillerApi.useListFillerSources({ query: { enabled: isAdmin } });

  // Catalog health (V35). Member-readable, so no `enabled` gate: it explains why a channel plays
  // what it plays, which is the same reason the catalog listing itself is member-visible.
  const poolQuery = fillerApi.useFillerPool();
  const pool = unwrap(poolQuery.data, (b) => b);

  // The Incoming queue (V35) — what has been downloaded but is not yet filed. Admin-only on the
  // server, so the QUERY is gated too, not just the tab: a member's request would 403 and fill
  // their console with an error about a surface they cannot reach.
  const incomingQuery = fillerApi.useFillerIncoming({ query: { enabled: isAdmin } });
  const incomingAsks = unwrap(incomingQuery.data, (b) => b.asks) ?? [];
  const incomingReels = unwrap(incomingQuery.data, (b) => b.reels) ?? [];
  const incomingTotal = unwrap(incomingQuery.data, (b) => b.total) ?? 0;

  // ⚠ Proposing a pull DOWNLOADS NOTHING — it writes a proposal for the approval queue (§10).
  // The toast says so, because a button that appears to start a download and does not is worse
  // than one that explains itself.
  const proposePull = fillerApi.useProposeFillerPull({
    mutation: {
      onSuccess: () => {
        toast.success("Pull proposed", {
          description: "Nothing is downloading yet. Approve it in the queue to start.",
        });
      },
      onError: (err) => {
        const problem = toProblem(err);
        toast.error(problem?.title ?? "Couldn't plan a pull", {
          ...(problem?.detail ? { description: problem.detail } : {}),
        });
      },
    },
  });
  const fetchSource = fillerApi.useFetchFillerSource({
    mutation: {
      onSuccess: () => {
        // Both: the catalog changed AND the per-source counts derive from it.
        void queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerQueryKey() });
        void queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerSourcesQueryKey() });
      },
    },
  });

  const sync = fillerApi.useSyncFiller({ mutation: { onSuccess: invalidate } });
  const aiTag = fillerApi.useTagFiller({ mutation: { onSuccess: invalidate } });

  // Which clip a write is in flight for, so ONE row disables rather than the whole list. The
  // mutation's own isPending is global to the hook — using it alone greys out every button on
  // the page while a single confirm lands, which reads as the page having frozen.
  const [busyClip, setBusyClip] = useState<string>();

  // Bulk selection (V35). ⚠ Deliberately NOT in the URL, unlike the filters: a selection is a
  // transient intent about the rows in front of you, and a shared link that carried it would
  // hand someone else a pre-armed destructive action over clips they never chose.
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const toggleSelected = (path: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (!next.delete(path)) next.add(path);
      return next;
    });
  const clearSelection = () => setSelected(new Set());

  // Bulk retag. Each field is independent on the server, so sending only what the operator
  // picked leaves the other two alone rather than blanking them.
  const bulkTag = fillerApi.useBulkTagFiller({
    mutation: {
      onSuccess: (res) => {
        clearSelection();
        invalidate();
        void queryClient.invalidateQueries({ queryKey: fillerApi.getFillerIncomingQueryKey() });
        void queryClient.invalidateQueries({ queryKey: fillerApi.getFillerPoolQueryKey() });
        const updated = isOk(res) ? res.data.updated : 0;
        toast.success(`Retagged ${pluralize(updated, "clip")}`);
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't retag those clips"),
    },
  });

  // Removing a clip from the catalog (V35). ⚠ A TOMBSTONE on the server: the clip leaves the
  // catalog and stops being used in breaks, and the file is untouched. The copy here says
  // "catalog" for that reason and must keep saying it.
  const removeClips = fillerApi.useBulkRemoveFiller({
    mutation: {
      onSuccess: () => {
        setBusyClip(undefined);
        invalidate();
        void queryClient.invalidateQueries({ queryKey: fillerApi.getFillerIncomingQueryKey() });
        void queryClient.invalidateQueries({ queryKey: fillerApi.getFillerPoolQueryKey() });
      },
      onError: (e) => {
        setBusyClip(undefined);
        toast.error(toProblem(e).title ?? "Couldn't remove those clips");
      },
    },
  });

  // Era-suggestion confirm (§10 V34). ⚠ The PATCH body must carry the clip's CURRENT
  // audience/category: UpdateClipTags writes all three columns unconditionally, so a
  // bare `{era}` would wipe the other two. Setting era confirms and clears the
  // suggestion in the same write (the BE's rule).
  const confirmEra = fillerApi.useTagFillerClip({
    mutation: {
      onSuccess: () => {
        setBusyClip(undefined);
        invalidate();
        // The Incoming queue and the pool strip both change when a clip is filed, and neither
        // is keyed on the clip list — without these the row stays in the queue until a reload.
        void queryClient.invalidateQueries({ queryKey: fillerApi.getFillerIncomingQueryKey() });
        void queryClient.invalidateQueries({ queryKey: fillerApi.getFillerPoolQueryKey() });
      },
      onError: (e) => {
        setBusyClip(undefined);
        toast.error(toProblem(e).title ?? "Couldn't confirm the era");
      },
    },
  });

  // Curried so the JSX spread stays a plain value — a typed inline arrow inside
  // `{...(cond ? {...} : {})}` trips the TSX parser on the generic-looking annotation.
  const cycleFor = (clip: ClipDTO) => (change: TagChange) => retag(clip, change);

  // ⚠ THE ONLY WAY THIS PAGE WRITES ONE TAG. `UpdateClipTags` overwrites era, audience AND
  // category on every call, so a PATCH carrying just the field being changed silently wipes
  // the other two. Cycling a chip makes that a per-click hazard rather than a once-per-dialog
  // one, so the whole tag row is assembled HERE from the clip and the single change — no
  // call site gets to remember or forget the siblings.
  const retag = (clip: ClipDTO, change: TagChange) =>
    confirmEra.mutate({
      id: clip.path,
      data: {
        // Kind is deliberately absent: the BE writes it separately (a shared code path with
        // the AI tagger), and sending it here would be a second opinion on a field this
        // interaction never edits.
        era: change.era ?? clip.era,
        audience: (change.audience ?? clip.audience) as never,
        category: change.category ?? clip.category,
      },
    });

  // Compilation splitting (§10 V34): POST starts a detection JOB, the terminal
  // `filler_split` SSE frame hands over the proposal id, and we navigate to the review
  // gate. Same shape as the ingest job below — request returns immediately, progress
  // arrives on the bus.
  const [splitJob, setSplitJob] = useState<{
    clipPath: string;
    jobId: string;
    status: string;
    error?: string;
  }>();
  const split = fillerApi.useSplitFiller({
    mutation: {
      onSuccess: (res, vars) => {
        if (isOk(res)) setSplitJob({ clipPath: vars.id, jobId: res.data.jobId, status: "running" });
      },
    },
  });

  useLoomarrEventListener({
    onFillerSplit: (e) => {
      // Frames for OTHER split jobs (another tab, another admin) are not ours to act on.
      if (!e.jobId || e.jobId !== splitJob?.jobId) return;
      if (e.status === "success" && e.proposalId) {
        setSplitJob(undefined);
        void navigate({ to: "/filler/splits/$proposalId", params: { proposalId: e.proposalId } });
      } else if (e.status === "error") {
        setSplitJob((prev) => (prev ? { ...prev, status: "error", error: e.error } : prev));
      }
    },
  });

  if (!fillerConfigured) {
    return (
      <div className="flex flex-col gap-6 p-6">
        <PageHeading />
        <EmptyState
          title="No filler folder configured"
          description="Add a folder of commercials, bumpers, and station IDs under Settings → Defaults. Loomarr indexes it for scheduling; point Tunarr at the same folder so it can play the clips."
          {...(isAdmin
            ? {
                action: {
                  label: "Open filler defaults",
                  onClick: () => navigate({ to: "/settings/defaults" }),
                },
              }
            : {})}
        />
      </div>
    );
  }

  const rows = clips.data?.status === 200 ? (clips.data.data.clips ?? []) : undefined;
  const clipList = rows ?? [];
  const sourceRows = unwrap(sourcesQuery.data, (b) => b.sources) ?? [];

  const filtered = Boolean(q || kind || audience || untagged);

  // The header's at-a-glance line (the mock's `watchLine`). ⚠ Counts the UNFILTERED catalog
  // where it can: `clipList` is the current query's result, so with a filter applied it
  // would read "1 clip" on a catalog of 200 — a header that changes meaning when you type
  // in the search box is worse than no header. Sources are admin-only, so a member sees the
  // clip count alone rather than a "0 sources" that is really "you can't see them".
  const statusLine = [
    ...(isAdmin && sourceRows.length > 0 ? [pluralize(sourceRows.length, "source")] : []),
    filtered ? `${pluralize(clipList.length, "clip")} shown` : pluralize(clipList.length, "clip"),
  ].join(" · ");

  return (
    // ⚠ p-6 — the page owns its own gutter. Without it this page rendered flush against the
    // sidebar and the right edge (heading at x=224 where every other page sits at 248), so
    // the tab rule and every card ran edge to edge. The shell deliberately adds no padding
    // (a full-bleed page like the Guide needs none), which makes it each page's job.
    <div className="flex flex-col gap-6 p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <PageHeading status={statusLine} />
        {isAdmin && (
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={sync.isPending}
              onClick={() => sync.mutate()}
              title="Re-scan the drop-folder (Loomarr probes it directly since §9.1, no Tunarr needed)"
            >
              <RefreshCw aria-hidden />
              {sync.isPending ? "Syncing…" : "Sync"}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={aiTag.isPending || !features?.suggestions}
              onClick={() => aiTag.mutate()}
              // AI tagging classifies era/audience/category from text signals (§10) and
              // needs the LLM the Suggest tab uses; without it the button would 409.
              title={
                features?.suggestions
                  ? "Classify untagged commercials with the configured LLM"
                  : "Connect an LLM under Settings → AI to auto-tag clips"
              }
            >
              <Sparkles aria-hidden />
              {aiTag.isPending ? "Tagging…" : "AI tag"}
            </Button>
          </div>
        )}
      </div>

      {/* Catalog health, above the tabs rather than inside one (V35).
          ⚠ A strip, not a tab: what the catalog holds is the CONTEXT the other tabs are read
          in, and as its own tab it was the thing nobody clicked — which is how an install ends
          up with four hundred clips and channels still falling back to the bumper card. */}
      {pool && (
        <PoolHealth
          pool={pool}
          {...(isAdmin ? { onProposePull: () => proposePull.mutate({ data: {} }) } : {})}
          proposing={proposePull.isPending}
        />
      )}

      {/* Three tabs, matching the redesigned mock: Catalog · Incoming · Sources.
          ⚠ There is no Discover tab any more. Finding clips used to be its own destination; it
          is now something you do TO a source, which is the only place the answer differs. An old
          ?tab=discover bookmark falls back to Catalog rather than rendering a blank panel. */}
      <CountTabs
        label="Filler sections"
        tabs={[
          { id: "catalog", label: "Catalog", count: clipList.length },
          ...(isAdmin ? [{ id: "incoming", label: "Incoming", count: incomingTotal }] : []),
          { id: "sources", label: "Sources", count: sourceRows.length },
        ]}
        activeId={tab}
        onSelect={(id) => setFilters({ tab: id === "catalog" ? undefined : id })}
      />

      {tab === "incoming" && isAdmin ? (
        <div
          id="panel-incoming"
          role="tabpanel"
          aria-labelledby="tab-incoming"
          className="flex flex-col gap-4"
        >
          {incomingQuery.error != null && (
            <ErrorState error={incomingQuery.error} onRetry={() => incomingQuery.refetch()} />
          )}
          <IncomingPanel
            asks={incomingAsks}
            reels={incomingReels}
            // ⚠ Carries the clip's CURRENT audience and category, not just the era. The BE's
            // UpdateClipTags writes all three columns unconditionally, so a bare `{era}` would
            // silently wipe the other two — the hazard the comment above `confirmEra` records,
            // and one this call site reproduced on its first draft.
            onConfirmEra={(ask) => {
              setBusyClip(ask.path);
              confirmEra.mutate({
                id: ask.path,
                data: {
                  era: ask.suggestedEra ?? 0,
                  ...(ask.audience ? { audience: ask.audience as never } : {}),
                  ...(ask.category ? { category: ask.category } : {}),
                },
              });
            }}
            onEditTags={(ask) => setTagging(ask.path)}
            // "Don't use it" removes the clip from the CATALOG. The file stays where the
            // operator put it — the server's action is a tombstone, never a delete.
            onDismiss={(ask) => {
              setBusyClip(ask.path);
              removeClips.mutate({ data: { paths: [ask.path] } });
            }}
            {...(confirmEra.isPending || removeClips.isPending ? { busyPath: busyClip } : {})}
          />
          {/* The download tooling still lives here — it is how clips ARRIVE, which is what this
              tab is about. It moved off the retired Discover tab rather than being deleted. */}
          <IngestPanel ingestAvailable={Boolean(features?.ingest)} onIngested={invalidate} />
        </div>
      ) : tab === "sources" ? (
        <div id="panel-sources" role="tabpanel" aria-labelledby="tab-sources">
          <FillerSources
            sources={sourceRows}
            total={unwrap(sourcesQuery.data, (b) => b.total) ?? 0}
            onFetch={() => fetchSource.mutate()}
            fetching={fetchSource.isPending ? "folder" : null}
            error={fetchSource.error?.detail ?? sourcesQuery.error?.detail ?? null}
          />
        </div>
      ) : (
        <div id="panel-catalog" role="tabpanel" aria-labelledby="tab-catalog" className="flex flex-col gap-6">
          {sync.error != null && <ErrorState error={sync.error} />}
          {aiTag.error != null && <ErrorState error={aiTag.error} />}
          {split.error != null && <ErrorState error={split.error} />}
          {clips.error != null && <ErrorState error={clips.error} onRetry={() => clips.refetch()} />}

          {/* The bulk bar, shown only when something is selected. It appears ABOVE the grid
              rather than floating over it: a bar that covers the cards hides the very thing the
              operator is deciding about. */}
          {selected.size > 0 && (
            <div className="flex flex-wrap items-center gap-3 rounded-lg border border-signal/40 bg-signal/5 p-3">
              <span className="font-mono text-signal text-xs">
                {pluralize(selected.size, "clip")} selected
              </span>

              {BULK_TAG_FIELDS.map((field) => (
                <Select
                  key={field.key}
                  value=""
                  onValueChange={(value) =>
                    bulkTag.mutate({
                      data: {
                        paths: [...selected],
                        [field.key]: field.key === "era" ? Number(value) : value,
                      },
                    })
                  }
                >
                  <SelectTrigger className="w-auto min-w-32" aria-label={field.label}>
                    <SelectValue placeholder={field.label} />
                  </SelectTrigger>
                  <SelectContent>
                    {field.options.map((option) => (
                      <SelectItem key={option} value={option}>
                        {option}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ))}

              {/* ⚠ "Remove from catalog", not "Delete". The server's action is a TOMBSTONE: the
                  clip stops appearing here and stops being used in breaks, and the file stays
                  exactly where the operator put it. The label has to keep saying catalog. */}
              <Button
                variant="outline"
                size="sm"
                disabled={removeClips.isPending}
                onClick={() => removeClips.mutate({ data: { paths: [...selected] } })}
                title="Stop using these clips. The files stay in your folder."
              >
                Remove from catalog
              </Button>

              <Button variant="ghost" size="sm" className="ml-auto" onClick={clearSelection}>
                Clear
              </Button>
            </div>
          )}

          {/* Split detection progress (§10 V34) — a job, not a request, so it gets a live
              status line the way ingest does. Success NAVIGATES to the review route; only
              running/error render here. */}
          {splitJob && (
            <p
              role="status"
              className={
                splitJob.status === "error" ? "text-onair-300 text-sm" : "text-muted-foreground text-sm"
              }
            >
              {splitJob.status === "error"
                ? (splitJob.error ?? "Split detection failed.")
                : `Detecting cuts in ${splitJob.clipPath}… this can take a few minutes for a long compilation.`}
            </p>
          )}

          {sync.data?.status === 200 && (
            <p className="text-muted-foreground text-sm">
              {`Synced: ${sync.data.data.added} added, ${sync.data.data.updated} updated, ${sync.data.data.pruned} pruned.`}
            </p>
          )}
          {aiTag.data?.status === 200 && (
            <p className="text-muted-foreground text-sm">
              {`Tagged ${aiTag.data.data.tagged} of ${aiTag.data.data.considered} considered${
                aiTag.data.data.partial ? `, ${aiTag.data.data.partial} partially` : ""
              }.`}
            </p>
          )}

          <Card className="flex flex-wrap items-end gap-3 p-4">
            {/* ⚠ Capped. `flex-1` alone stretched a clip-name box to ~900px on a 1440
                viewport — a text field far wider than anything typed into it, which reads
                as a layout bug rather than a generous input. It still grows on narrow
                screens (min-w-48) and stops being silly on wide ones. */}
            <div className="min-w-48 max-w-md flex-1">
              <Label htmlFor="clip-search">Search</Label>
              <Input
                id="clip-search"
                value={q}
                placeholder="Clip name"
                onChange={(e) => setFilters({ q: e.target.value || undefined })}
              />
            </div>
            <div>
              <Label htmlFor="clip-kind">Kind</Label>
              {/* "any" sentinel ↔ "" (Radix forbids an empty value) — the no-filter default. */}
              <Select
                value={kind || "any"}
                onValueChange={(v) => setFilters({ kind: v === "any" ? undefined : v })}
              >
                <SelectTrigger id="clip-kind">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="any">Any</SelectItem>
                  <SelectItem value="commercial">Commercial</SelectItem>
                  <SelectItem value="bumper">Bumper</SelectItem>
                  <SelectItem value="station_id">Station ID</SelectItem>
                  <SelectItem value="psa">PSA</SelectItem>
                  <SelectItem value="trailer">Trailer</SelectItem>
                  <SelectItem value="interstitial">Interstitial</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="clip-audience">Audience</Label>
              <Select
                value={audience || "any"}
                onValueChange={(v) => setFilters({ audience: v === "any" ? undefined : v })}
              >
                <SelectTrigger id="clip-audience">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="any">Any</SelectItem>
                  <SelectItem value="kids">Kids</SelectItem>
                  <SelectItem value="family">Family</SelectItem>
                  <SelectItem value="general">General</SelectItem>
                  <SelectItem value="late_night">Late night</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Button
              variant={untagged ? "default" : "outline"}
              size="sm"
              onClick={() => setFilters({ untagged: untagged ? undefined : true })}
              // "Untagged" means a COMMERCIAL missing a match tag — bumpers do their job
              // without era/audience, so they are never counted as needing work (§10).
              title="Commercials missing era, audience, or category"
            >
              Untagged only
            </Button>
          </Card>

          {rows === undefined ? (
            <p className="text-muted-foreground text-sm">Loading clips…</p>
          ) : rows.length === 0 ? (
            <EmptyState
              title={filtered ? "No clips match" : "No clips yet"}
              description={
                filtered
                  ? "Try a wider filter, or clear the search."
                  : "Anything that lands in the filler folder shows up here on its own. Drop files in, or let Discover fetch them."
              }
              {...(filtered
                ? {
                    action: {
                      label: "Clear filters",
                      onClick: () =>
                        setFilters({
                          q: undefined,
                          kind: undefined,
                          audience: undefined,
                          untagged: undefined,
                        }),
                    },
                  }
                : // The mock's empty state offers "Find clips" rather than only describing
                  // the folder — an empty catalog is exactly when an operator needs the way
                  // OUT of it, and the tab is otherwise a thing they have to notice.
                  isAdmin
                  ? { action: { label: "Find clips", onClick: () => setFilters({ tab: "discover" }) } }
                  : {})}
            />
          ) : (
            <>
              <p className="text-muted-foreground text-sm">{pluralize(rows.length, "clip")}</p>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {rows.map((clip) => (
                  <ClipCard
                    key={clip.path}
                    clip={clip}
                    {...(isAdmin ? { onTag: () => setTagging(clip.path) } : {})}
                    {...(isAdmin && clip.aiTagged ? { onConfirmTags: () => setTagging(clip.path) } : {})}
                    {...(isAdmin ? { onPin: () => setPinning(clip.path) } : {})}
                    {...(isAdmin && clip.suggestedEra
                      ? { onConfirmEra: () => retag(clip, { era: clip.suggestedEra ?? 0 }) }
                      : {})}
                    {...(isAdmin ? { onCycle: cycleFor(clip) } : {})}
                    {...(isAdmin ? { onSplit: () => split.mutate({ id: clip.path }) } : {})}
                    splitPending={splitJob?.clipPath === clip.path && splitJob.status === "running"}
                    {...(isAdmin ? { onToggleSelect: () => toggleSelected(clip.path) } : {})}
                    selected={selected.has(clip.path)}
                  />
                ))}
              </div>
            </>
          )}

          {tagging && rows && (
            <ClipTagDialog
              clip={rows.find((c) => c.path === tagging)}
              onClose={() => setTagging(undefined)}
              onSaved={() => {
                setTagging(undefined);
                void invalidate();
              }}
            />
          )}

          {pinning && rows && (
            <PinClipDialog
              clip={rows.find((c) => c.path === pinning)}
              onClose={() => setPinning(undefined)}
            />
          )}
        </div>
      )}
    </div>
  );
};

// PageHeading — orients the two-surface model the per-channel filler feature introduced:
// this page is the CATALOG (every clip, its tags), while each channel CHOOSES from it on
// its own Filler section (theme + pin/exclude, with a live preview). Tagging here is what
// makes a clip matchable there; the link makes that relationship navigable rather than
// implicit — the cohesion gap that made filler feel "disjointed".
// ⚠ `status` is the mock's `watchLine` MINUS its "last scan 2m ago" clause. No DTO carries a
// last-scan time (FillerSourceDTO is {configured,count,detail,fetchable,kind,remotes,target}),
// and inventing one from the client's clock would assert a scan happened when the page
// merely loaded. Two thirds of a true line beats three thirds of a plausible one.
const PageHeading = ({ status }: { status?: string }) => (
  <div>
    <div className="flex flex-wrap items-baseline gap-x-3">
      <h1 className="font-semibold text-xl">Filler</h1>
      {status && <span className="font-mono text-static-400 text-xs">{status}</span>}
    </div>
    <p className="mt-1 max-w-2xl text-muted-foreground text-sm">
      Your whole library of commercials, bumpers, and station IDs. Browse and tag them here. Tags are what let
      the scheduler match a clip to a channel. Each channel then{" "}
      <Link to="/guide" className="text-signal underline-offset-2 hover:underline">
        picks and previews its own filler
      </Link>{" "}
      from this catalog.
    </p>
  </div>
);

export { FillerPage };
