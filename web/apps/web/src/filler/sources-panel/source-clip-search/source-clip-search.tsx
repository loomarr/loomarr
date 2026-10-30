import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";
import type { FillerSourcePreviewItemDTO } from "@loomarr/api/models/fillerSourcePreviewItemDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { SourceSearch } from "@/components/loomarr/filler/source-search";
import { Disclosure } from "@/components/ui/disclosure";
import { SourceItemPreviewDialog } from "../source-item-preview-dialog";
import { useQueuedClipJobs } from "../use-queued-clip-jobs";

interface SourceClipSearchProps {
  /** An Archive.org source whose actions include `search`. Remount per source (`key`). */
  source: FillerSourceDTO;
}

// "Find a specific clip": search one Archive.org collection, preview a hit, queue it for download.
// Owns the whole flow — the query, the lazily measured runtimes, the queued jobs and the preview
// dialog — so none of it re-renders the rest of the drawer on a keystroke.
const SourceClipSearch = ({ source }: SourceClipSearchProps) => {
  const { isAdmin } = useAuth();
  const [open, setOpen] = useState(false);
  const [preview, setPreview] = useState<FillerSourcePreviewItemDTO>();
  const queuedJobs = useQueuedClipJobs(source.id);

  // Searching a source for clips to add (V35). ⚠ `submittedQuery` is a SEPARATE state from what the
  // operator is typing: the query runs on submit, not on keystroke. Each search costs archive.org
  // one Solr request plus a metadata call per row, so firing per character would be both slow and
  // rude — and the results would flicker under the cursor.
  const [query, setQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");
  const discover = fillerApi.useDiscoverFiller(
    { q: submittedQuery, collection: source.uri },
    // Admin-only on the server, and only once something has actually been submitted — an
    // enabled query with an empty q would 422 on mount.
    {
      query: {
        enabled:
          isAdmin && submittedQuery.trim().length >= 2 && source.kind === "archive" && Boolean(source.uri),
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
  const stats = unwrap(statsQuery.data, (b) => b.stats) ?? {};

  return (
    <>
      <Disclosure open={open} onOpenChange={setOpen} className="border-border border-t pt-5">
        <Disclosure.SectionTrigger
          title="Find a specific clip"
          description="Search this Archive.org collection by title or keyword."
        />
        <Disclosure.Panel className="pt-4">
          <SourceSearch
            results={unwrap(discover.data, (b) => b.items) ?? []}
            total={unwrap(discover.data, (body) => body.total) ?? undefined}
            stats={stats}
            onVisible={(visible) =>
              setStatIds((previous) => {
                const next = visible.filter((id) => !previous.includes(id));
                return next.length > 0 ? [...previous, ...next] : previous;
              })
            }
            loadingStats={statsQuery.isFetching ? statIds : []}
            query={query}
            onQueryChange={setQuery}
            onSearch={() => {
              setSubmittedQuery(query);
              setStatIds([]);
            }}
            onPreview={(clip) =>
              setPreview({
                title: clip.title || clip.id,
                url: clip.url,
                durationMs: stats[clip.id]?.durationMs ?? clip.durationMs,
              })
            }
            onQueue={queuedJobs.queue}
            queueStatus={queuedJobs.queueStatus}
            queueing={queuedJobs.queueing ?? null}
            searching={discover.isFetching}
            error={discover.error?.detail ?? queuedJobs.error ?? null}
          />
        </Disclosure.Panel>
      </Disclosure>
      <SourceItemPreviewDialog item={preview} kind="archive" onClose={() => setPreview(undefined)} />
    </>
  );
};

export { SourceClipSearch };
