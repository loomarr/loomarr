import type { ClipDTO } from "@loomarr/api";
import { fillerApi, isOk, settingsApi, toProblem, unwrap } from "@loomarr/api";
import { formatRelative, pluralize } from "@loomarr/core";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { LayoutGrid, List } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { useAuth } from "@/auth";
import {
  ClipCard,
  ClipPlayer,
  ClipRow,
  EmptyState,
  ErrorState,
  PoolHealth,
  WatchPill,
} from "@/components/loomarr";
import {
  Button,
  Card,
  Input,
  Label,
  NavTabs,
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
import { IncomingTab } from "../incoming-tab";
import { PinClipDialog } from "../pin-clip-dialog";
import { SourcesPanel } from "../sources-panel";
import { useFillerInvalidate } from "../use-filler-invalidate";
import type { FillerPageProps } from "./filler-page.type";

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

// The catalog's two renderings (V35b, the mock's `catViews`). Grid is the default because
// filler is picked by LOOKING at it — a wall of frames is the point; the list is for scanning
// a large catalog and bulk-selecting, where a thumbnail per row is noise.
//
// ⚠ Only the GRID acts on one clip (tag/pin/split). That is the mock's split and it is
// deliberate: see the note in `clip-row.tsx` on why per-row actions are not added back.
const VIEWS = [
  { id: "grid", label: "Grid", icon: LayoutGrid, title: "Cards with thumbnails and per-clip actions" },
  { id: "list", label: "List", icon: List, title: "A dense row per clip, for scanning and selecting" },
] as const;

// FillerPage — the §10 clip catalog: browse, search, tag and (on the filler image) download.
// ⚠ No longer "sync": V38c removed the whole-catalog Sync and AI-tag buttons, because both run
// on their own schedule and a button for work that already happens invites the reading that it
// does not happen unless pressed. Filtering is client-driven but server-executed: the store indexes
// these columns, so a query per filter change is cheaper and always correct, versus
// holding thousands of clips in memory to filter locally (§7.2 — no client index).
// ⚠ `tab` is a PROP, not read from the URL here (V-nav-paths): which section shows is now
// the PATH (`/filler`, `/filler/incoming`, `/filler/sources`), and three different route
// files render this component — each hands over the section it owns. The catalog FILTERS
// below stay URL-driven, and stay scoped to `/_authed/filler/` because only that route
// validates them; Incoming and Sources carry none of them (see filler-page's own note on
// `catalogSearch`).
const FillerPage = ({ tab }: FillerPageProps) => {
  useDocumentTitle("Filler");
  const navigate = useNavigate();
  const { isAdmin } = useAuth();
  // Filters live in the URL (deep-linkable, shareable, back-button aware) — the route's
  // validateSearch narrows them. setFilters merges a partial change and writes with
  // `replace: true` so typing in the search box doesn't stack a history entry per keystroke.
  // ⚠ `strict: false`, not `from: "/_authed/filler/"`: that route id only matches when the
  // active leaf IS the catalog, and this component also mounts under `/filler/incoming` and
  // `/filler/sources`, neither of which declares these params. Loosely-typed search still
  // reads whatever the catalog route wrote, so a link back to Catalog with `q`/`kind`/etc.
  // round-trips; the other two tabs just never populate them.
  const {
    q = "",
    kind = "",
    audience = "",
    untagged = false,
    view = "grid",
  } = useSearch({ strict: false }) as Partial<FillerSearch>;
  const setFilters = (next: Partial<FillerSearch>) =>
    navigate({ to: "/filler", search: (prev) => ({ ...prev, ...next }), replace: true });

  // What the Catalog tab's link carries back.
  //
  // ⚠ It preserves the CATALOG's own state (the filters and the grid/list choice) — the path
  // itself (`/filler`) is what used to be `tab=catalog`; the query is only ever these filters
  // now. The Incoming and Sources links deliberately carry NONE of it: `q`, `kind` and
  // `audience` filter the clip grid, and nothing on those tabs is a clip — dragging a search
  // term onto Sources would leave a filter applied to a list it cannot narrow.
  const catalogSearch = {
    ...(q ? { q } : {}),
    ...(kind ? { kind } : {}),
    ...(audience ? { audience } : {}),
    ...(untagged ? { untagged } : {}),
    ...(view !== "grid" ? { view } : {}),
  };
  const [tagging, setTagging] = useState<string>();
  const [pinning, setPinning] = useState<string>();
  // The clip open in the player (V39), by path. A path rather than the DTO so the dialog always
  // renders the CURRENT row — retagging a clip while it plays would otherwise leave the player
  // showing a stale copy.
  const [playing, setPlaying] = useState<string>();

  const settings = settingsApi.useSettingsList();
  const features = unwrap(settings.data, (b) => b.features);
  const fillerConfigured = Boolean(features?.filler);

  const clips = fillerApi.useListFiller({
    ...(q ? { q } : {}),
    ...(kind ? { kind: kind as never } : {}),
    ...(audience ? { audience: audience as never } : {}),
    ...(untagged ? { untagged: true } : {}),
  });

  // ⚠ `invalidateLifecycle` invalidates the clip list AND the two queue views (Incoming, pool).
  // The trio was written out by hand at four call sites here, which is three chances to forget
  // the third key and leave an operator looking at a queue that has already moved on.
  const { invalidateCatalog: invalidate, invalidateLifecycle } = useFillerInvalidate();

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

  // The header's live status (§10 V38c). ⚠ Member-readable, deliberately: it carries counts and a
  // verdict but no paths, targets or ids, so a member sees whether filler is working without
  // being shown the infrastructure behind it. Gating this on `isAdmin` would leave every member
  // with a permanently grey indicator on a healthy install.
  const watch = unwrap(fillerApi.useFillerWatch().data, (b) => b);

  // Catalog health (V35). Member-readable, so no `enabled` gate: it explains why a channel plays
  // what it plays, which is the same reason the catalog listing itself is member-visible.
  const poolQuery = fillerApi.useFillerPool();
  const pool = unwrap(poolQuery.data, (b) => b);

  // The Incoming queue's COUNT (V35) — the tab badge, which the shell owns because the badge is
  // visible from every tab. Admin-only on the server, so the QUERY is gated too, not just the
  // tab: a member's request would 403 and fill their console with an error about a surface they
  // cannot reach.
  //
  // ⚠ Only `total` is read here. The queue's contents (asks, reels, recently-filed) belong to
  // `IncomingTab`, which runs the same query when it mounts — TanStack dedupes them onto one
  // cache entry, so this is one request, not two. What it is NOT is the previous arrangement,
  // where the Catalog tab unpacked and held a queue it never rendered.
  //
  // ⚠ `total` deliberately EXCLUDES the recently-filed audit list: that is not work waiting,
  // and a tab badge that never clears stops being read.
  const incomingQuery = fillerApi.useFillerIncoming({ query: { enabled: isAdmin } });
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
  // ⚠ The filing mutations (file / hold / confirm-era) and their busy-row state moved into
  // `IncomingTab` with the queue they serve. They are not shared: nothing outside Incoming
  // files or holds a clip, so mounting them here made the Catalog tab pay for a surface it
  // never touches.

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
  // "Select all" over the rows currently RENDERED (V35b). ⚠ Scoped to the filtered set on
  // purpose: `rows` is what the operator can see, and the bulk bar's next control is
  // "Remove from catalog". Selecting clips hidden behind a filter would arm a destructive
  // action over rows nobody looked at.
  //
  // It doubles as the un-select once everything shown is picked, so the control is not a
  // one-way door that needs a second button to undo.
  const allSelected = (rows: readonly ClipDTO[]) =>
    rows.length > 0 && rows.every((clip) => selected.has(clip.path));
  const selectAll = (rows: readonly ClipDTO[]) =>
    setSelected(allSelected(rows) ? new Set() : new Set(rows.map((clip) => clip.path)));

  // Bulk retag. Each field is independent on the server, so sending only what the operator
  // picked leaves the other two alone rather than blanking them.
  const bulkTag = fillerApi.useBulkTagFiller({
    mutation: {
      onSuccess: (res) => {
        clearSelection();
        invalidateLifecycle();
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
      onSuccess: invalidateLifecycle,
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't remove those clips"),
    },
  });

  // Era-suggestion confirm (§10 V34). ⚠ The PATCH body must carry the clip's CURRENT
  // audience/category: UpdateClipTags writes all three columns unconditionally, so a
  // bare `{era}` would wipe the other two. Setting era confirms and clears the
  // suggestion in the same write (the BE's rule).
  // ⚠ `invalidateLifecycle`, not just the clip list. A retag can move a clip between the queue
  // and the catalog and changes what the pool can cover, so all three views are now stale —
  // without the other two keys the row sits in Incoming until a reload.
  const confirmEra = fillerApi.useTagFillerClip({
    mutation: {
      onSuccess: invalidateLifecycle,
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't confirm the era"),
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

  // The header's at-a-glance line (the mock's `watchLine`), rendered entirely from
  // `GET /v1/filler/watch`.
  //
  // ⚠ **Every clause comes from the SERVER, including the counts.** They were computed here from
  // `/v1/filler/sources` — which is admin-only, so a member's header silently lost its source
  // clause — and the clip count came from `clipList`, the CURRENT QUERY's result, so a filter
  // matching nothing rendered "0 clips" on a catalog of two hundred. A header that changes
  // meaning when you type in the search box is worse than no header.
  //
  // ⚠ The "last scan" clause is DROPPED when the server sends no timestamp, rather than rendered
  // as "never". Most sources are scanned rather than fetched, so absent is the ordinary case —
  // "never" would read as a fault on a drop-folder working exactly as intended.
  // ⚠ **"N waiting" is a SEPARATE clause, not folded into the clip count.** Auto-fetch holds
  // everything it downloads until someone reviews it, so a first fetch leaves the catalog at zero
  // with a full Incoming queue. Reporting the catalog alone rendered "5 of 5 sources on · 0 clips"
  // on an install that had just pulled twelve — the fetcher's success reading as a failure, and
  // the reason the maintainer asked "why don't I see any clips in the catalog then?". Summing them
  // instead would be the opposite lie: it would claim a channel can play clips nobody has approved.
  const statusLine = watch
    ? [
        `${watch.sourcesOn} of ${watch.sourcesTotal} sources on`,
        pluralize(watch.clips, "clip"),
        ...(watch.held > 0 ? [`${watch.held} waiting`] : []),
        ...(watch.lastScanAt ? [`last scan ${formatRelative(watch.lastScanAt)}`] : []),
      ].join(" · ")
    : "";

  return (
    // ⚠ p-6 — the page owns its own gutter. Without it this page rendered flush against the
    // sidebar and the right edge (heading at x=224 where every other page sits at 248), so
    // the tab rule and every card ran edge to edge. The shell deliberately adds no padding
    // (a full-bleed page like the Guide needs none), which makes it each page's job.
    <div className="flex flex-col gap-6 p-6">
      {/* ⚠ The Sync and AI-tag buttons were HERE and are deliberately gone (V38c, maintainer).
          Neither was work an operator should have to remember to start: the sync runs on its own
          schedule and drains the watch folder every pass, and tagging runs after it. A button for
          something that already happens invites the reading that it does NOT happen unless
          pressed. Both remain reachable where they mean something — "Fetch now" on an individual
          Sources row, and the per-clip tag dialog.

          The header is now the mock's `watchLine` pill alone: what is on, what is held, when
          anything last arrived. */}
      {/* ⚠ `items-start` and a 16px gap, matching the mock: the pill aligns to the TOP of the
          heading block, not to its baseline or centre. The heading takes the remaining width so
          the pill sits hard against the right edge. */}
      <div className="flex flex-wrap items-start gap-4">
        <div className="min-w-0 flex-1">
          <PageHeading />
        </div>
        {/* ⚠ Everything here comes from `GET /v1/filler/watch` — the counts AND the verdict.
            An earlier pass derived the health in the browser from `/v1/filler/sources`, which
            was wrong twice: that route is admin-only, so a member's dot sat permanently grey on
            a working install, and the rule ("every source dark", "nothing for days") is domain
            logic that belongs where it can be tested against the store. */}
        {watch && <WatchPill status={statusLine} health={watch.health} />}
      </div>

      {/* Catalog health, above the tabs rather than inside one (V35).
          ⚠ A strip, not a tab: what the catalog holds is the CONTEXT the other tabs are read
          in, and as its own tab it was the thing nobody clicked — which is how an install ends
          up with four hundred clips and channels still falling back to the bumper card.
          ⚠ HIDDEN on Sources, matching the mock's own `showPool: fillerTab !== 'sources'`. The
          strip answers "what does my catalog hold and can it fill a break" — which is the context
          for reading CLIPS. The Sources tab is about where clips come from, and a coverage strip
          above it invites the reading that a source is at fault for a channel's weak coverage. */}
      {pool && tab !== "sources" && (
        <PoolHealth
          pool={pool}
          {...(isAdmin ? { onProposePull: () => proposePull.mutate({ data: {} }) } : {})}
          proposing={proposePull.isPending}
        />
      )}

      {/* Three tabs, matching the redesigned mock: Catalog · Incoming · Sources.
          ⚠ There is no Discover tab any more. Finding clips used to be its own destination; it
          is now something you do TO a source, which is the only place the answer differs. */}
      {/* ⚠ Real LINKS, not buttons calling `navigate()`. Each tab is its own PATH now
          (V-nav-paths: `/filler`, `/filler/incoming`, `/filler/sources` — it used to be
          `?tab=`), so rendering it as a button would cost middle-click, copy-link and every
          browser affordance for nothing. `NavTabs` is shared with Queue and Settings. */}
      <NavTabs
        label="Filler sections"
        linkComponent={Link}
        // ⚠ Sources carries NO count. Catalog and Incoming are queues — "how many clips do I
        // have", "how many need me" — and the number is the reason to look. The Sources tab is a
        // destination: the count of configured sources is already the header pill's job ("4 of 5
        // sources on"), and a bare "5" beside the label answers a question nobody asked while
        // implying the tab is a backlog.
        tabs={[
          { id: "catalog", label: "Catalog", to: "/filler", search: catalogSearch, count: clipList.length },
          ...(isAdmin
            ? [{ id: "incoming", label: "Incoming", to: "/filler/incoming", count: incomingTotal }]
            : []),
          { id: "sources", label: "Sources", to: "/filler/sources" },
        ]}
        activeId={tab}
      />

      {tab === "incoming" && isAdmin ? (
        // ⚠ The tab owns its own query and its four filing mutations — see `incoming-tab.tsx`.
        // They used to live up here, so the Catalog tab mounted the whole Incoming queue too.
        <IncomingTab onEditTags={setTagging} />
      ) : tab === "sources" ? (
        <SourcesPanel sources={sourceRows} sourcesError={sourcesQuery.error?.detail} />
      ) : (
        <div id="panel-catalog" role="tabpanel" aria-labelledby="tab-catalog" className="flex flex-col gap-6">
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

          {/* ⚠ The "Synced: N added…" and "Tagged N of M…" result banners were here and went with
              their buttons (V38c). Reporting the outcome of work nothing on this page can start
              is a result with no cause — an operator reading it has no way to know what produced
              it or how to produce it again. Per-source outcomes belong on the Sources row that
              did the work. */}

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

            {/* Grid ⇄ list (V35b, the mock's `catViews`). ⚠ A radiogroup, not two toggle
                buttons: these are two states of ONE setting, and a pair of independent
                buttons announces neither which is active nor that they are alternatives.
                `ml-auto` puts it at the far end of the toolbar, as the mock draws it. */}
            <div className="ml-auto flex gap-1" role="radiogroup" aria-label="Clip view">
              {VIEWS.map((v) => (
                <Button
                  key={v.id}
                  variant={view === v.id ? "default" : "outline"}
                  size="sm"
                  role="radio"
                  aria-checked={view === v.id}
                  title={v.title}
                  onClick={() => setFilters({ view: v.id === "grid" ? undefined : v.id })}
                >
                  <v.icon className="size-4" aria-hidden />
                  {v.label}
                </Button>
              ))}
            </div>
          </Card>

          {rows === undefined ? (
            <p className="text-muted-foreground text-sm">Loading clips…</p>
          ) : rows.length === 0 ? (
            <EmptyState
              title={filtered ? "No clips match" : "No clips yet"}
              description={
                filtered
                  ? "Try a wider filter, or clear the search."
                  : "Anything that lands in the filler folder shows up here on its own. Drop files in, or ask Loomarr to pull some."
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
                : // An empty catalog is exactly when an operator needs the way OUT of it, so the
                  // empty state carries the same action the health strip does.
                  //
                  // ⚠ It used to read "Find clips" and navigate to `tab: "discover"` — a tab this
                  // phase RETIRED. `validateSearch` drops the unknown value, so the button landed
                  // back on the empty catalog it was offered from: a control that looked like the
                  // way out and did nothing. Two independent reviewers found it, which is the
                  // useful lesson — deleting a destination is not done until every route TO it is
                  // gone, and a nav target is not type-checked.
                  isAdmin
                  ? {
                      action: {
                        label: "Propose a pull",
                        onClick: () => proposePull.mutate({ data: {} }),
                      },
                    }
                  : {})}
            />
          ) : (
            <>
              <div className="flex items-center gap-3">
                <p className="text-muted-foreground text-sm">{pluralize(rows.length, "clip")}</p>
                {/* Select all (V35b, the mock's `catSelAll`). ⚠ It selects the FILTERED rows —
                    what is on screen — not the whole catalog. Selecting rows the operator
                    cannot see, then offering "Remove from catalog", is how a bulk action
                    surprises someone. The label says so when a filter is active. */}
                {isAdmin && (
                  <Button variant="ghost" size="sm" onClick={() => selectAll(rows)}>
                    {allSelected(rows) ? "Clear selection" : filtered ? "Select these" : "Select all"}
                  </Button>
                )}
              </div>
              {view === "list" ? (
                <div className="overflow-hidden rounded-lg border border-border">
                  {rows.map((clip) => (
                    <ClipRow
                      key={clip.path}
                      clip={clip}
                      {...(isAdmin ? { onToggleSelect: () => toggleSelected(clip.path) } : {})}
                      selected={selected.has(clip.path)}
                    />
                  ))}
                </div>
              ) : (
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
                      // ⚠ NOT gated on isAdmin, unlike every other action on this card. Watching
                      // a clip mutates nothing, and `/v1/filler/media` is member-readable by
                      // design — these are the same commercials the household's channels play at
                      // them. Gating it would hide a safe capability from exactly the people who
                      // would want to check what is airing.
                      onPlay={() => setPlaying(clip.path)}
                    />
                  ))}
                </div>
              )}
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

          {/* The player (V39). ⚠ `?? null` rather than a `&&` guard like its siblings above: this
              dialog takes a NULLABLE clip and derives `open` from it, so handing it `undefined`
              would be a type error and handing it nothing at all would leave it permanently
              closed. A row that has vanished under a filter closes the player, which is the
              honest outcome — the clip it was showing is no longer in the list. */}
          <ClipPlayer
            clip={rows?.find((c) => c.path === playing) ?? null}
            onClose={() => setPlaying(undefined)}
          />
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
