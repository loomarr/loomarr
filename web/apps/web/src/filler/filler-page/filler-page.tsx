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
  DiscoverPanel,
  EmptyState,
  ErrorState,
  FillerSources,
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

  // Find clips (V33). ⚠ Two pieces of query state, not one: `discoverQuery` is what the
  // operator is TYPING and `discoverSubmitted` is what they SEARCHED for. Firing on every
  // keystroke would send a request per character to a public API we do not own.
  const [discoverQuery, setDiscoverQuery] = useState("");
  const [discoverSubmitted, setDiscoverSubmitted] = useState("");
  const discover = fillerApi.useDiscoverFiller(
    { q: discoverSubmitted },
    // enabled: only after a real search. retry: false because a 502 here means archive.org
    // is not answering, and retrying three times just delays telling the operator that.
    { query: { enabled: discoverSubmitted !== "", retry: false } },
  );
  const discovered = unwrap(discover.data, (b) => b.items) ?? [];
  const discoverTotal = unwrap(discover.data, (b) => b.total) ?? 0;
  const discoverNote = unwrap(discover.data, (b) => b.licenceNote);

  // Adding a discovered clip is an INGEST, not a new mechanism: the id becomes the archive.org
  // URL the existing downloader already accepts. Reusing that path means one place understands
  // how a download is started, reported and retried.
  const ingestIds = fillerApi.useIngestFiller({ mutation: { onSuccess: invalidate } });

  // ⚠ Every hook stays ABOVE the `fillerConfigured` early return below. Placing these two
  // after it skipped them on the unconfigured path and crashed the page with "rendered more
  // hooks than during the previous render" — the whole catalog replaced by an error boundary.
  //
  // Sources are admin-only on the server, so a member's request would 403. Gating the QUERY
  // (not just the tab) keeps a member's console clean.
  const sourcesQuery = fillerApi.useListFillerSources({ query: { enabled: isAdmin } });
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

  // Era-suggestion confirm (§10 V34). ⚠ The PATCH body must carry the clip's CURRENT
  // audience/category: UpdateClipTags writes all three columns unconditionally, so a
  // bare `{era}` would wipe the other two. Setting era confirms and clears the
  // suggestion in the same write (the BE's rule).
  const confirmEra = fillerApi.useTagFillerClip({
    mutation: {
      onSuccess: invalidate,
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't confirm the era"),
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

  return (
    // ⚠ p-6 — the page owns its own gutter. Without it this page rendered flush against the
    // sidebar and the right edge (heading at x=224 where every other page sits at 248), so
    // the tab rule and every card ran edge to edge. The shell deliberately adds no padding
    // (a full-bleed page like the Guide needs none), which makes it each page's job.
    <div className="flex flex-col gap-6 p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <PageHeading />
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

      {/* Three tabs, matching the v2 mock: Catalog · Discover · Sources. Discover was a
          CARD stacked under the catalog, which put finding clips, the clip list and an
          image caveat on the page as three equal slabs with no hierarchy. The mock makes
          it a peer of Catalog because it is a different task, not more of the same one. */}
      <CountTabs
        label="Filler sections"
        tabs={[
          { id: "catalog", label: "Catalog", count: clipList.length },
          ...(isAdmin ? [{ id: "discover", label: "Discover", count: discovered.length }] : []),
          { id: "sources", label: "Sources", count: sourceRows.length },
        ]}
        activeId={tab}
        onSelect={(id) => setFilters({ tab: id === "catalog" ? undefined : id })}
      />

      {tab === "discover" && isAdmin ? (
        <div id="panel-discover" role="tabpanel" aria-labelledby="tab-discover" className="flex flex-col gap-4">
          <DiscoverPanel
            items={discovered}
            total={discoverTotal}
            licenceNote={discoverNote}
            query={discoverQuery}
            onQueryChange={setDiscoverQuery}
            onSearch={() => setDiscoverSubmitted(discoverQuery.trim())}
            searching={discover.isFetching}
            searched={discoverSubmitted !== ""}
            {...(features?.ingest
              ? { onAdd: (ids: string[]) => ingestIds.mutate({ data: { urls: ids.map(archiveURL) } }) }
              : {})}
          />
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
                      ? {
                          onConfirmEra: () =>
                            confirmEra.mutate({
                              id: clip.path,
                              data: {
                                era: clip.suggestedEra ?? 0,
                                audience: clip.audience ?? "",
                                category: clip.category ?? "",
                              },
                            }),
                        }
                      : {})}
                    {...(isAdmin ? { onSplit: () => split.mutate({ id: clip.path }) } : {})}
                    splitPending={splitJob?.clipPath === clip.path && splitJob.status === "running"}
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

// archiveURL turns a discovered id into the URL ingest takes. ⚠ Built here rather than trusted
// from the API's `url` field: that one is the item's human-facing details page, and while the
// two happen to coincide today, ingest resolving a display URL would be a coincidence rather
// than a contract.
const archiveURL = (id: string) => `https://archive.org/details/${encodeURIComponent(id)}`;

// PageHeading — orients the two-surface model the per-channel filler feature introduced:
// this page is the CATALOG (every clip, its tags), while each channel CHOOSES from it on
// its own Filler section (theme + pin/exclude, with a live preview). Tagging here is what
// makes a clip matchable there; the link makes that relationship navigable rather than
// implicit — the cohesion gap that made filler feel "disjointed".
const PageHeading = () => (
  <div>
    <h1 className="font-semibold text-xl">Filler</h1>
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
