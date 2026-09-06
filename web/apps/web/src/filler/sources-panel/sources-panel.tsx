import * as fillerApi from "@loomarr/api/endpoints/filler";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { useAuth } from "@/auth/use-auth";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { FillerSources } from "@/components/loomarr/filler/filler-sources";
import { SourceSearch } from "@/components/loomarr/filler/source-search";
// ⚠ No `Card` and no `Label`. Add-a-source is a plain block under a single top rule (the mock
// draws no box), and its fields are labelled by their per-kind PLACEHOLDER plus `aria-label` —
// a static visible label above an input whose meaning changes with the kind would contradict it.
import { Button } from "@/components/ui/button";
import { Caption } from "@/components/ui/caption";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
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
  archive: { label: "Collection or URL", placeholder: "classic_tv_commercials" },
  youtube: { label: "Playlist or channel URL", placeholder: "youtube.com/playlist?list=…" },
} as const;

// SourcesPanel — the Sources tab of the filler page (§10 V35/V37/V38c): registered sources
// (folders, libraries, archive.org, YouTube), each switchable, fetchable and (if `removable`)
// forgettable, plus per-source search for archive.org and the "Add a source" form.
const SourcesPanel = ({ sources, sourcesError }: SourcesPanelProps) => {
  const queryClient = useQueryClient();
  const { isAdmin } = useAuth();

  // The per-source search expander (V35b). Local rather than in the URL: an open panel with an
  // empty query is not a view worth sharing, and the search TERM is already transient here.
  //
  // ⚠ **Which source's panel is open, not WHETHER one is (§10 V54 B6).** This was a single
  // boolean shared by every row, and `renderSearch` is called once per source — so pressing
  // "Search it" on one archive collection expanded the panel on EVERY searchable row at once,
  // each showing the same query, the same results and the same "Close" button. With one
  // collection registered it looked correct; the roll-up this phase renders is precisely what
  // puts several of them on screen together.
  const [searchOpenFor, setSearchOpenFor] = useState<string>();

  const invalidate = () => queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerQueryKey() });

  const fetchSource = fillerApi.useFetchFillerSource({
    mutation: {
      onSuccess: () => {
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
  // Which source's "Fetch now" is running. The request is row-scoped, and this local state keeps
  // the corresponding card busy while the bounded acquisition and catalog scan complete.
  const [fetchingSource, setFetchingSource] = useState<string>();
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
  const [newSourceKind, setNewSourceKind] = useState<"folder" | "library" | "archive" | "youtube">("library");
  const [newSourceURI, setNewSourceURI] = useState("");
  const [newSourceCountry, setNewSourceCountry] = useState("");
  const [newSourceMarket, setNewSourceMarket] = useState("");
  const addSource = fillerApi.useAddFillerSource({
    mutation: {
      onSuccess: () => {
        setNewSourceURI("");
        setNewSourceCountry("");
        setNewSourceMarket("");
        toast.success("Source added", { description: "Loomarr will check it on its download schedule." });
        void queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerSourcesQueryKey() });
      },
    },
  });

  const [removingSource, setRemovingSource] = useState<string>();
  const removeSource = fillerApi.useDeleteFillerSource({
    mutation: {
      onSettled: () => setRemovingSource(undefined),
      onSuccess: () => {
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
  const searchedSource = sources.find((source) => source.id === searchOpenFor);
  const discover = fillerApi.useDiscoverFiller(
    { q: submittedQuery, collection: searchedSource?.uri },
    // Admin-only on the server, and only once something has actually been submitted — an
    // enabled query with an empty q would 422 on mount.
    {
      query: {
        enabled:
          isAdmin &&
          submittedQuery.trim().length >= 2 &&
          searchedSource?.kind === "archive" &&
          Boolean(searchedSource.uri),
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

  // Opening a DIFFERENT source's search is a new search, so the query and the result set start
  // clean rather than showing the previous row's answers under this row's name.
  //
  // ⚠ One panel open at a time, which is what lets the query state below stay single. Several
  // open panels would each need their own query, submitted term and `statIds` — and `statIds`
  // spends a real ~1.8s archive.org call per id, so two result sets on screen would quietly
  // double that.
  const toggleSearch = (id: string) => {
    if (searchOpenFor === id) {
      setSearchOpenFor(undefined);
      return;
    }
    setSourceQuery("");
    setSubmittedQuery("");
    setStatIds([]);
    setSearchOpenFor(id);
  };

  // Which results have been queued this session. ⚠ Session state, deliberately NOT derived from
  // the catalog: a queued download has not landed yet, so the clip it becomes is not in the
  // catalog to compare against, and the row must still report that the operator already asked.
  const [queuedIds, setQueuedIds] = useState<string[]>([]);
  const [queueingId, setQueueingId] = useState<string>();
  const queueClip = fillerApi.useIngestFiller({
    mutation: {
      onSettled: () => setQueueingId(undefined),
      onSuccess: () => {
        // ⚠ The id comes from the state set at CLICK time, not from the mutation's variables:
        // the request body carries a URL, and mapping it back to a row would mean re-deriving
        // an identity the click already knew.
        setQueuedIds((prev) => (queueingId ? [...prev, queueingId] : prev));
        // Downloads land in the drop-folder and appear on the next scan, so the catalog and the
        // per-source counts both become stale.
        invalidate();
      },
    },
  });

  const discoveredResults = unwrap(discover.data, (b) => b.items) ?? [];

  return (
    <div className="flex flex-col gap-6">
      <FillerSources
        sources={sources}
        // ⚠ The pending row is tracked by ID. This used to be
        // `fetching={fetchSource.isPending ? "folder" : null}` — a hardcoded row — so
        // fetching ANY source lit the drop-folder's spinner. Invisible while the folder was
        // the only fetchable row; a visible lie now that V38c allows many.
        onFetch={(id) => {
          setFetchingSource(id);
          fetchSource.mutate({ params: { id } });
        }}
        fetching={fetchSource.isPending ? fetchingSource : null}
        onToggleEnabled={(id, enabled) => {
          setTogglingSource(id);
          toggleSource.mutate({ id, data: { enabled } });
        }}
        toggling={toggleSource.isPending ? togglingSource : null}
        onRemove={(id) => {
          setRemovingSource(id);
          removeSource.mutate({ id });
        }}
        removing={removeSource.isPending ? removingSource : null}
        error={toggleSource.error?.detail ?? fetchSource.error?.detail ?? sourcesError ?? null}
        // Finding clips is something you do TO a source (V35), so the search lives INSIDE
        // the row that owns it rather than in a detached card below the list (V35b, the
        // mock's `sv.showSearch`). ⚠ Searching downloads nothing; `Queue download` is the
        // only path that fetches, and it goes through the SAME ingest route the manual URL
        // box uses rather than a second downloader.
        //
        // ⚠ Reads the server's `searchable` flag (V37) rather than testing the kind here.
        // V35b hard-coded `kind === "remote"` and noted that flattening would change which
        // row matched; it did, and a client-side kind test would have silently searched the
        // wrong row — or nothing. The server owns which sources have something to query:
        // archive can be searched, a YouTube playlist can only be ENUMERATED by yt-dlp, and
        // a search box there would return nothing forever.
        //
        // Still gated on `enabled` too: offering search on a source the operator just
        // switched off would contradict the switch's own copy ("stops scanning, searching
        // and downloading from it").
        renderSearch={(source) =>
          source.searchable && source.enabled ? (
            <div className="mt-2 w-full border-border border-t pt-3">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => toggleSearch(source.id)}
                aria-expanded={searchOpenFor === source.id}
                // ⚠ The accessible name carries the SOURCE. With several collections on screen
                // under one provider, five buttons all reading "Search it" are indistinguishable
                // to anyone not looking at the row they sit in.
                aria-label={
                  searchOpenFor === source.id
                    ? `Close the search of ${source.target}`
                    : `Search ${source.target}`
                }
              >
                {searchOpenFor === source.id ? "Close" : "Search it"}
              </Button>
              {searchOpenFor === source.id && (
                <div className="mt-3">
                  <SourceSearch
                    results={discoveredResults}
                    total={unwrap(discover.data, (b) => b.total) ?? undefined}
                    stats={discoveredStats}
                    // ⚠ De-duplicated HERE, not in the component: the observer reports the
                    // visible set on every scroll, and each id it repeats is a real ~1.8s
                    // upstream request.
                    onVisible={(visible) =>
                      setStatIds((prev) => {
                        const next = visible.filter((id) => !prev.includes(id));
                        return next.length > 0 ? [...prev, ...next] : prev;
                      })
                    }
                    loadingStats={statsQuery.isFetching ? statIds : []}
                    query={sourceQuery}
                    onQueryChange={setSourceQuery}
                    onSearch={() => {
                      setSubmittedQuery(sourceQuery);
                      // A new search is a new result set: keeping the old ids would ask
                      // archive.org about rows nobody is looking at any more.
                      setStatIds([]);
                    }}
                    onQueue={(clip) => {
                      setQueueingId(clip.id);
                      queueClip.mutate({ data: { urls: [clip.url] } });
                    }}
                    queued={queuedIds}
                    queueing={queueClip.isPending ? queueingId : null}
                    searching={discover.isFetching}
                    error={discover.error?.detail ?? queueClip.error?.detail ?? null}
                  />
                </div>
              )}
            </div>
          ) : undefined
        }
      />

      {/* Add a source (V37, the mock's "Add a source"). ⚠ The KIND select is what makes this
          work at all: an archive identifier and a YouTube URL are validated by incompatible
          rules on the server, so before V37 the hardcoded archive validator rejected every
          playlist URL with a message about archive.org collections. The operator says which
          kind they mean rather than the server guessing from the string. */}
      {/* ⚠ NOT a Card. The mock draws this as a plain block separated from the list by a single
          top rule (`border-top: #1B1E24`, 16px above) — it continues the Sources section rather
          than sitting in a box of its own. An earlier pass boxed it, which read as a second,
          competing panel below the list. */}
      {isAdmin && (
        <div className="flex flex-col gap-3 border-border/40 border-t pt-4">
          <h3 className="font-medium text-sm">Add a source</h3>
          <form
            className="flex flex-wrap items-center gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (!newSourceURI.trim()) return;
              addSource.mutate({
                data: {
                  kind: newSourceKind,
                  uri: newSourceURI.trim(),
                  country: newSourceCountry.trim().toUpperCase(),
                  market: newSourceMarket.trim(),
                },
              });
            }}
          >
            {/* ⚠ No visible field labels — the mock has none, and the per-kind PLACEHOLDER is the
                label ("Library name on Jellyfin — e.g. Commercials"). It changes with the kind, so
                a static label above it would either repeat it or contradict it. `aria-label` keeps
                both controls named for screen readers, which is what the visible label was for. */}
            <Select
              value={newSourceKind}
              onValueChange={(v) => {
                const kind = v as typeof newSourceKind;
                setNewSourceKind(kind);
                if (kind === "folder" || kind === "library") {
                  setNewSourceCountry("");
                  setNewSourceMarket("");
                }
              }}
            >
              <SelectTrigger id="new-source-kind" className="w-45" aria-label="Kind of source to add">
                <SelectValue />
              </SelectTrigger>
              {/* The mock's `sourceKinds`, in its order. ⚠ `library` is here because V38c restored
                  library SCANNING (§10) — V35 had removed the kind's work entirely, so offering it
                  then would have added a row nothing acted on. */}
              <SelectContent>
                <SelectItem value="library">Media server library</SelectItem>
                <SelectItem value="folder">Watched folder</SelectItem>
                <SelectItem value="archive">Internet Archive</SelectItem>
                <SelectItem value="youtube">Playlist / collection URL</SelectItem>
              </SelectContent>
            </Select>
            {/* ⚠ MONO, per the mock: what goes in here is a path, an identifier or a URL — text
                where character-level accuracy matters and a proportional font hides a typo. */}
            <Input
              id="new-source-uri"
              className="min-w-64 flex-1 font-mono"
              value={newSourceURI}
              placeholder={SOURCE_KIND_COPY[newSourceKind].placeholder}
              aria-label={SOURCE_KIND_COPY[newSourceKind].label}
              onChange={(e) => setNewSourceURI(e.target.value)}
            />
            {(newSourceKind === "archive" || newSourceKind === "youtube") && (
              <>
                <Input
                  className="w-24 font-mono uppercase"
                  value={newSourceCountry}
                  maxLength={2}
                  placeholder="US"
                  aria-label="Source country code"
                  onChange={(e) => setNewSourceCountry(e.target.value)}
                />
                <Input
                  className="min-w-40 flex-1"
                  value={newSourceMarket}
                  placeholder="Market (optional)"
                  aria-label="Source local market"
                  disabled={!newSourceCountry.trim()}
                  onChange={(e) => setNewSourceMarket(e.target.value)}
                />
              </>
            )}
            <Button type="submit" variant="outline" disabled={addSource.isPending || !newSourceURI.trim()}>
              {addSource.isPending ? "Adding…" : "+ Add source"}
            </Button>
          </form>
          {addSource.error != null && <ErrorState error={addSource.error} />}
          {/* Scheduled source polling is real work (§10 V38b), while every arrival remains held
              until terminal admission releases it. Name both halves so a successful fetch does not
              look broken merely because Catalog correctly excludes its Incoming clips. */}
          <Caption>
            Country-only sources are nationwide; add a market for local sources. Once installation geography
            is configured, unclassified and out-of-market sources are not fetched. Source selection controls
            acquisition only; every clip still needs certified safety, rights, and playback evidence.
          </Caption>
        </div>
      )}
    </div>
  );
};

export { SourcesPanel };
