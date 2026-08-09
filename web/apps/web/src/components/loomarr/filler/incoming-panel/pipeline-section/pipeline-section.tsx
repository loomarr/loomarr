import type { IncomingPipelineDTO } from "@loomarr/api";
import { clipThumbURL, formatClipDuration, pluralize } from "@loomarr/core";
import { Caption, Disclosure } from "@/components/ui";
import { ClipPipeline, copyFor } from "../../clip-pipeline";

// The "what is Loomarr doing right now" half of Incoming (§10 V51b/V51e).
//
// ⚠ This section is the whole reason V51b's pipeline was worth building. Before it, a download of
// forty commercials said "waiting to be checked" for up to an hour while three cron jobs worked on
// it at :15, :30 and :50 — the system was working and looked broken. Nothing here changes what the
// machine does; it changes whether an operator can see it.
//
// ⚠ **Collapsed by default, and the collapsed summary is the thing that advances.** Forty clips ×
// eight rungs is 320 lines of moving text. The pip strip carries the stage-by-stage watch for the
// whole queue at once, and expanding one row gives the named ladder, the skip reasons and the
// percentage. Rows are never auto-expanded on a frame — that makes the queue jump under the
// cursor of someone reading it.

// The sentence under the name. ⚠ Deliberately the ACTIVE voice for a running rung ("Working out
// what it is") and not a status word: the queue's job is to answer "is anything happening", and
// "tag / running" answers it in the machine's vocabulary rather than the operator's.
const sentenceFor = (row: IncomingPipelineDTO): string => {
  const { active, label } = copyFor(row.stage);
  if (row.status === "running") return active;
  if (row.status === "queued") return `Waiting to ${label.toLowerCase()}`;
  if (row.status === "failed") return `${label} — failed, retrying`;
  return label;
};

const PipelineRow = ({ ladder, row }: { ladder: string[]; row: IncomingPipelineDTO }) => {
  const name = row.name || row.hash;

  return (
    <li>
      <Disclosure className="rounded-lg border border-border">
        <div className="flex flex-wrap items-center gap-3 p-3">
          {/* ⚠ The trigger is a DISCRETE chevron rather than the whole row, because the row is
              going to carry its own controls. CollapsibleSection wraps its header in one <button>,
              which would make anything else in here unreachable by keyboard. */}
          <Disclosure.Trigger label={`Show what is happening to ${name}`} />

          {/* ⚠ Rendered only when the row says a thumbnail EXISTS. A clip still at `probe` has
              none, and firing an <img> at /v1/filler/thumb/{hash} on the strength of the hash
              alone would draw a broken image for exactly the newest rows — the ones an operator
              is most likely to be watching. */}
          {row.thumbnail && (
            <img
              src={clipThumbURL(row.hash)}
              alt=""
              className="h-8 w-14 shrink-0 rounded object-cover"
              loading="lazy"
            />
          )}

          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <div className="flex flex-wrap items-baseline gap-x-2">
              <span className="truncate font-medium text-sm">{name}</span>
              {/* ⚠ formatClipDuration, NOT formatDuration — the latter is minute-granular because
                  it formats programme runtimes, and every filler clip would render "0m". */}
              {row.durationMs != null && row.durationMs > 0 && (
                <Caption className="tabular-nums">{formatClipDuration(row.durationMs)}</Caption>
              )}
            </div>
            <Caption>{sentenceFor(row)}</Caption>
          </div>

          <ClipPipeline row={row} ladder={ladder} className="shrink-0" />
        </div>

        <Disclosure.Panel className="border-border border-t p-3">
          <ClipPipeline row={row} ladder={ladder} variant="list" />
        </Disclosure.Panel>
      </Disclosure>
    </li>
  );
};

const PipelineSection = ({ ladder, rows }: { ladder: string[]; rows: IncomingPipelineDTO[] }) => {
  // ⚠ **EXACTLY ONE live region for the whole section**, carrying only the most recent transition.
  // A `role="status"` per row — or per rung — is the "chorus of live regions" frontend-design §5.3
  // forbids: forty clips announcing themselves is not forty times as informative, it is unusable.
  // The newest row is chosen by `updatedAt`, which the server already orders the work by.
  const newest = rows.reduce<IncomingPipelineDTO | undefined>(
    (best, r) => (!best || r.updatedAt > best.updatedAt ? r : best),
    undefined,
  );

  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-col gap-0.5">
        <h2 className="font-medium text-sm">Loomarr is preparing {pluralize(rows.length, "clip")}</h2>
        <Caption>These aren't ready to play yet. Nothing here needs you — it's just working.</Caption>
      </div>

      <p role="status" className="sr-only">
        {newest ? `${newest.name || newest.hash}: ${sentenceFor(newest)}` : ""}
      </p>

      <ul className="flex flex-col gap-2">
        {rows.map((row) => (
          <PipelineRow key={row.hash} row={row} ladder={ladder} />
        ))}
      </ul>
    </section>
  );
};

export { PipelineSection, sentenceFor };
