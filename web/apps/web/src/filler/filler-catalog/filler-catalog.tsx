import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { pluralize } from "@loomarr/core/format";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useRef, useState } from "react";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { ClipCard } from "@/components/loomarr/filler/clip-card";
import { ClipRow } from "@/components/loomarr/filler/clip-row";
import { Button } from "@/components/ui/button";
import { usePhoneWidth } from "@/lib/use-phone-width";
import { cn } from "@/lib/utils";
import { ConfirmSplitDialog } from "../confirm-split-dialog";
import type { FillerSearch } from "../filler-search";
import { CatalogBulkBar } from "./catalog-bulk-bar";
import { CatalogToolbar } from "./catalog-toolbar";
import type { CatalogView } from "./catalog-view-toggle";
import { ClipInspectorSheet } from "./clip-inspector-sheet";
import { CompositeCatalogGroup } from "./composite-catalog-group";
import type { FillerCatalogProps } from "./filler-catalog.type";
import { useCatalogSelection } from "./use-catalog-selection";
import { useClipSplit } from "./use-clip-split";

// How many clips one page of the catalog holds (§10 V51d).
//
// ⚠ **ONE size for both the grid and the list**, deliberately. A per-view size would renumber the
// pages under the operator the moment they switched rendering — page 3 of the grid and page 3 of
// the list would be different clips — and `pageSize` is kept out of the URL for the same reason.
// 60 divides evenly by the grid's 2/3/4 columns, so the last row is never a ragged single card.
const CATALOG_PAGE_SIZE = 60;

// FillerPage — the §10 clip catalog: browse, search, tag and (on the filler image) download.
// ⚠ No longer "sync": V38c removed the whole-catalog Sync and AI-tag buttons, because both run
// on their own schedule and a button for work that already happens invites the reading that it
// does not happen unless pressed. Filtering is client-driven but server-executed: the store indexes
// these columns, so a query per filter change is cheaper and always correct, versus
// holding thousands of clips in memory to filter locally (§7.2 — no client index).
// The catalog is mounted only for `/filler/library`. Its filters remain URL-driven, deep-linkable,
// and scoped to the validated filler route; the page shell merely preserves that opaque search
// state when it renders the Library navigation link.
const FillerCatalog = ({ isAdmin }: FillerCatalogProps) => {
  const navigate = useNavigate();
  // Filters live in the URL (deep-linkable, shareable, back-button aware) — the route's
  // validateSearch narrows them. setFilters merges a partial change and writes with
  // `replace: true` so typing in the search box doesn't stack a history entry per keystroke.
  // ⚠ `strict: false` is retained at this composition boundary because the route id is generated
  // and the shell preserves the validated search object opaquely. The cast below narrows only to
  // the route's exported search contract; Incoming and Sources never mount this module.
  const {
    q = "",
    kind = "",
    audience = "",
    taxon = "",
    unclassified = false,
    withoutAxis,
    untagged = false,
    view = "grid",
    page = 1,
    parent: parentHash,
  } = useSearch({ strict: false }) as Partial<FillerSearch>;
  const filtered = Boolean(q || kind || audience || taxon || unclassified || withoutAxis || untagged);
  const resultScope = JSON.stringify({
    q,
    kind,
    audience,
    taxon,
    unclassified,
    withoutAxis,
    untagged,
    parentHash,
    page,
  });
  // ⚠ **Every filter change RESETS the page, and this is the single highest-risk line on the
  // page** (§10 V51d). `setFilters` merges blindly; without the reset, typing in the search box
  // while on page 7 lands on an empty page 7 of a two-page result and renders "No clips match"
  // over a catalog that matches plenty. The rule lives HERE, in the one function every filter
  // control calls, rather than at each control — six call sites is six chances to forget it.
  //
  // ⚠ Paging itself is the exception, so it passes `page` explicitly and keeps it.
  const setFilters = (next: Partial<FillerSearch>) =>
    navigate({
      to: "/filler/library",
      search: (prev) => ({ ...prev, page: undefined, ...next }),
      replace: true,
    });
  const chooseView = (next: CatalogView) =>
    navigate({
      to: "/filler/library",
      search: (prev) => ({
        ...prev,
        page: page > 1 ? page : undefined,
        view: next === "grid" ? undefined : next,
      }),
      replace: true,
    });
  // A phone gets the folded toolbar and the 2-up compact grid (#1817, decided on #1908). It's a
  // render decision, not just an arrangement: the compact card drops its chips and aired line.
  const phone = usePhoneWidth();

  const [inspecting, setInspecting] = useState<string>();
  const detailTrigger = useRef<HTMLElement>(null);
  const openDetails = (hash: string) => {
    detailTrigger.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setInspecting(hash);
  };
  // The clip whose split is awaiting confirmation, by hash (§10 V54 A8). Nothing fires until the
  // operator confirms — see the dialog for why the old first-click behaviour was a hazard.
  const [splitting, setSplitting] = useState<string>();
  const clipSplit = useClipSplit();
  const { job: splitJob } = clipSplit;

  // One page of the catalog (§10 V51d). ⚠ This was an unbounded read of every clip in the
  // install; the endpoint now caps at 500 and defaults to 100, so an un-paged catalog would
  // silently show the first hundred clips and call it the catalog.
  const clips = fillerApi.useListFiller(
    {
      ...(q ? { q } : {}),
      ...(kind ? { kind: kind as never } : {}),
      ...(audience ? { audience: audience as never } : {}),
      ...(taxon ? { taxon } : {}),
      ...(unclassified ? { unclassified: true } : {}),
      ...(withoutAxis ? { withoutAxis } : {}),
      ...(untagged ? { untagged: true } : {}),
      ...(parentHash ? { parentHash } : !filtered ? { includeComposites: true, topLevel: true } : {}),
      limit: CATALOG_PAGE_SIZE,
      ...(page > 1 ? { offset: (page - 1) * CATALOG_PAGE_SIZE } : {}),
    },
    { query: { enabled: true } },
  );
  const parent = fillerApi.useListFiller(
    { hashes: parentHash ? [parentHash] : [], includeComposites: true, limit: 1 },
    { query: { enabled: Boolean(parentHash) } },
  );

  // ⚠ The Discover query state that used to live here is GONE with its tab (V35). Searching a
  // source is moving onto the Sources tab, where it belongs — a search is something you do to a
  // source, not a destination of its own — and `GET /v1/filler/discover` is still the route
  // behind it. Deleting the state rather than leaving it wired to nothing: an unused query that
  // still fires is how a page keeps paying for a feature nobody can reach.

  const selection = useCatalogSelection(resultScope);
  // Paging (§10 V51d).
  //
  // ⚠ `replace: false`, unlike `setFilters`: paging IS navigation, so the back button should walk
  // back through the pages. Typing in a search box should not stack one history entry per
  // keystroke, which is why the two differ.
  //
  // ⚠ It clears the selection. "Select this page" means the rows in front of you, and the next
  // control on the bulk bar is Remove-from-catalog — a selection that survived paging would let
  // one click remove clips from a page nobody is looking at.
  const goToPage = (next: number) => {
    selection.clear();
    navigate({
      to: "/filler/library",
      search: (prev) => ({ ...prev, page: next > 1 ? next : undefined }),
    });
  };

  const rows = clips.data?.status === 200 ? (clips.data.data.clips ?? []) : undefined;
  const clipList = rows ?? [];
  // ⚠ `total` is how many clips MATCH THE FILTER, counted in SQL through the same predicate as
  // the rows (§10 V51d) — not `rows.length`, which is one page. The two are the same number only
  // on a single-page result, which is exactly why deriving one from the other looked fine for as
  // long as the listing was unbounded.
  const total = clips.data?.status === 200 ? (clips.data.data.total ?? 0) : 0;
  const pageCount = Math.max(1, Math.ceil(total / CATALOG_PAGE_SIZE));
  const firstOnPage = (page - 1) * CATALOG_PAGE_SIZE + 1;
  const lastOnPage = (page - 1) * CATALOG_PAGE_SIZE + clipList.length;

  const parentName = unwrap(parent.data, (body) => body.clips[0]?.name);
  const selectableRows = clipList.filter((clip) => !clip.isComposite);

  const renderCatalogRow = (clip: ClipDTO) => (
    <ClipRow
      key={clip.hash}
      clip={clip}
      {...(isAdmin ? { onToggleSelect: () => selection.toggle(clip.hash) } : {})}
      selected={selection.selected.has(clip.hash)}
      onOpen={() => openDetails(clip.hash)}
    />
  );
  const renderCatalogCard = (clip: ClipDTO) => (
    <ClipCard
      key={clip.hash}
      clip={clip}
      {...(isAdmin && clip.isComposite ? { onSplit: () => setSplitting(clip.hash) } : {})}
      splitPending={clipSplit.isRunning(clip.hash)}
      {...(isAdmin ? { onToggleSelect: () => selection.toggle(clip.hash) } : {})}
      selected={selection.selected.has(clip.hash)}
      onOpen={() => openDetails(clip.hash)}
      compact={phone}
    />
  );
  const renderComposite = (clip: ClipDTO) => (
    <CompositeCatalogGroup
      key={clip.hash}
      clip={clip}
      onManage={() => setFilters({ parent: clip.hash })}
      renderParent={view === "list" ? renderCatalogRow : renderCatalogCard}
      renderChild={(child) => (
        <ClipRow key={child.hash} clip={child} onOpen={() => openDetails(child.hash)} />
      )}
    />
  );

  return (
    <div className="flex flex-col gap-6">
      {clipSplit.error != null && <ErrorState error={clipSplit.error} />}
      {clips.error != null && <ErrorState error={clips.error} onRetry={() => clips.refetch()} />}

      {parentHash ? (
        <div className="border-border bg-card flex flex-wrap items-center gap-3 rounded-lg border p-3">
          <div className="min-w-0 flex-1">
            <p className="text-sm font-medium">{parentName || "Compilation segments"}</p>
            <p className="text-muted-foreground text-xs">Airable clips filed from this compilation.</p>
          </div>
          <Button variant="outline" size="sm" onClick={() => setFilters({ parent: undefined })}>
            Back to top-level catalog
          </Button>
        </div>
      ) : null}

      {taxon || unclassified || withoutAxis ? (
        <div className="border-signal/30 bg-signal/5 flex flex-wrap items-center gap-3 rounded-lg border p-3">
          <p className="min-w-0 flex-1 text-sm">
            {unclassified
              ? "Showing clips with no directly assigned taxonomy tags"
              : withoutAxis
                ? `Showing clips without a directly assigned ${withoutAxis} tag`
                : `Showing clips matching “${taxon}”, including descendants`}
          </p>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setFilters({ taxon: undefined, unclassified: undefined, withoutAxis: undefined })}
          >
            Clear taxonomy filter
          </Button>
        </div>
      ) : null}

      {selection.selected.size > 0 && (
        <CatalogBulkBar selected={selection.selected} onClear={selection.clear} />
      )}

      {/* Split detection progress (§10 V34) — a job, not a request, so it gets a live
          status line the way ingest does. Success NAVIGATES to the review route; only
          running/error render here. */}
      {splitJob && (
        <p
          role="status"
          className={splitJob.status === "error" ? "text-onair-300 text-sm" : "text-muted-foreground text-sm"}
        >
          {splitJob.status === "error"
            ? (splitJob.error ?? "Split detection failed.")
            : `Detecting cuts in ${splitJob.clipName}… this can take a few minutes for a long compilation.`}
        </p>
      )}

      {/* ⚠ The "Synced: N added…" and "Tagged N of M…" result banners were here and went with
          their buttons (V38c). Reporting the outcome of work nothing on this page can start
          is a result with no cause — an operator reading it has no way to know what produced
          it or how to produce it again. Per-source outcomes belong on the Sources row that
          did the work. */}

      <CatalogToolbar
        phone={phone}
        q={q}
        kind={kind}
        audience={audience}
        untagged={untagged}
        view={view}
        onFilterChange={setFilters}
        onViewChange={chooseView}
      />

      {rows === undefined ? (
        <p className="text-muted-foreground text-sm">Loading clips…</p>
      ) : rows.length === 0 ? (
        <EmptyState
          title={filtered ? "No clips match" : "No clips yet"}
          description={
            filtered
              ? "Try a wider filter, or clear the search."
              : "Add a source or use your own files. Loomarr prepares clips automatically and adds them here when they are ready."
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
                      taxon: undefined,
                      unclassified: undefined,
                      withoutAxis: undefined,
                      untagged: undefined,
                    }),
                },
              }
            : isAdmin
              ? {
                  action: {
                    label: "Open sources",
                    onClick: () => navigate({ to: "/filler/sources" }),
                  },
                }
              : {})}
        />
      ) : (
        <>
          {/* The count line sits ABOVE the grid ("did my filter work?"), the pager BELOW
              ("what's next?") — never both in one place. */}
          <div className="flex items-center gap-3">
            <p className="text-muted-foreground text-sm">
              {pageCount > 1
                ? `Showing ${firstOnPage.toLocaleString()}–${lastOnPage.toLocaleString()} of ${total.toLocaleString()}`
                : pluralize(total, "clip")}
            </p>
            {/* Select all (V35b, the mock's `catSelAll`). ⚠ It selects the FILTERED rows —
                what is on screen — not the whole catalog. Selecting rows the operator
                cannot see, then offering "Remove from catalog", is how a bulk action
                surprises someone. The label says so when a filter is active.
                ⚠ Paging sharpened that: "on screen" is now one PAGE, so the label says
                "Select this page" whenever there is more than one — and `goToPage` clears
                the selection, or a Remove would reach rows from a page nobody is looking at. */}
            {isAdmin && (
              <Button variant="ghost" size="sm" onClick={() => selection.selectAll(selectableRows)}>
                {selection.allSelected(selectableRows)
                  ? "Clear selection"
                  : pageCount > 1
                    ? "Select this page"
                    : filtered
                      ? "Select these"
                      : "Select all"}
              </Button>
            )}
          </div>
          {view === "list" ? (
            <div className="border-border overflow-hidden rounded-lg border">
              {rows.map((clip) => (clip.isComposite ? renderComposite(clip) : renderCatalogRow(clip)))}
            </div>
          ) : (
            <div className={cn("grid", phone ? "grid-cols-2 gap-2.5" : "grid-cols-2 gap-3 lg:grid-cols-3")}>
              {rows.map((clip) =>
                clip.isComposite ? (
                  <div key={clip.hash} className="col-span-full">
                    {renderComposite(clip)}
                  </div>
                ) : (
                  renderCatalogCard(clip)
                ),
              )}
            </div>
          )}

          {/* The pager sits BELOW the grid ("what's next?"); the count line above it answers
              "did my filter work?". Rendered only when there is more than one page, so the
              common household catalog never grows a control it does not need.

              ⚠ Prev/Next plus a position, not a numbered page strip. A strip is the V51e
              catalog redesign's call to make; what this must not do is leave the clips past
              row 60 unreachable, which is what a page size with no pager would be. */}
          {pageCount > 1 && (
            <nav aria-label="Catalog pages" className="flex items-center justify-between gap-3 pt-1">
              <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => goToPage(page - 1)}>
                Previous
              </Button>
              {/* ⚠ `aria-live="polite"`: paging replaces the grid in place, and without an
                  announcement a screen-reader user gets a silently different list. */}
              <p aria-live="polite" className="text-muted-foreground text-sm">
                Page {page.toLocaleString()} of {pageCount.toLocaleString()}
              </p>
              <Button
                variant="outline"
                size="sm"
                disabled={page >= pageCount}
                onClick={() => goToPage(page + 1)}
              >
                Next
              </Button>
            </nav>
          )}
        </>
      )}

      {/* The split confirmation (§10 V54 A8). Resolved from the CURRENT page like its
          siblings, so a clip that vanishes under a filter closes the dialog rather than
          confirming a hash that is no longer on screen. */}
      {splitting && rows && (
        <ConfirmSplitDialog
          clip={rows.find((c) => c.hash === splitting)}
          onConfirm={() => {
            const clip = rows.find((c) => c.hash === splitting);
            setSplitting(undefined);
            if (clip) clipSplit.start(clip);
          }}
          onClose={() => setSplitting(undefined)}
        />
      )}

      <ClipInspectorSheet
        hash={inspecting}
        isAdmin={isAdmin}
        finalFocus={detailTrigger}
        onClose={() => setInspecting(undefined)}
      />
    </div>
  );
};

export { FillerCatalog };
