import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FillerIncomingOutputBody } from "@loomarr/api/models/fillerIncomingOutputBody";
import type { IncomingClipDTO } from "@loomarr/api/models/incomingClipDTO";
import type { IncomingPipelineDTO } from "@loomarr/api/models/incomingPipelineDTO";
import { isOk } from "@loomarr/api/unwrap";
import type { FillerClipEvent } from "@loomarr/core/events";
import { useQueryClient } from "@tanstack/react-query";
import { useLoomarrEventListener } from "@/events/events-provider";

// useClipPipeline keeps the Incoming tab's pipeline rows current from the `filler_clip` SSE
// frame (§10 V51b), without a refetch per rung.
//
// ⚠ **The GET is truth; the frame is latency only** (§8, the events contract). Everything here is
// a repaint of data the server already sent — nothing is ASSEMBLED from frames. Three rules make
// that concrete, and each one is a bug this codebase has already paid for elsewhere:
//
//   1. **Never build the stage ladder from the sequence of frames received.** This bus drops
//      frames for a slow subscriber by design, so a ladder assembled from arrivals would be
//      silently missing rungs. Each frame is a self-sufficient snapshot — `FillerClipEvent`'s own
//      comment says so — and the ladder to draw against comes from the response's `stageOrder`.
//   2. **A frame for an unknown hash invalidates, it never inserts.** A row built from a frame
//      would be missing `stages`, `attempts` and `updatedAt`: every field the expanded view
//      renders. A half-row that looks like a row is worse than a row that arrives 200ms late.
//   3. **Terminal frames do not land here at all.** `events.ts` invalidates the whole `/v1/filler`
//      prefix the moment a clip stops running, because a filed clip changes the catalog too —
//      and the operator watching the catalog has no pipeline listener mounted.
const useClipPipeline = (): void => {
  const queryClient = useQueryClient();

  useLoomarrEventListener({
    onFillerClip: (frame) => {
      const key = fillerApi.getFillerIncomingQueryKey();

      // Terminal frames are the invalidating half (rule 3) — `events.ts` has already asked for a
      // refetch, so merging a snapshot on top of data that is about to be replaced would only
      // paint one frame of a state the server is mid-way through superseding.
      if (frame.disposition !== "running") return;

      queryClient.setQueryData(key, (prev: unknown) => {
        if (!isOk(prev as { data: unknown; status: number } | undefined)) return prev;
        const res = prev as { data: FillerIncomingOutputBody; status: 200 };
        const rows = res.data.clips ?? [];

        // ⚠ Matched by `find` rather than by index. `noUncheckedIndexedAccess` makes `rows[i]`
        // possibly-undefined, and the tempting fix — a `if (!rows[i]) return prev` after a
        // successful findIndex — is a guard that can never fire, which reads to the next person
        // as a case that CAN happen.
        const current = rows.find((r) => r.hash === frame.hash);
        // ⚠ A clip with no `pipeline` block is treated exactly like an unknown hash. It is either
        // brand new or pre-V51b, and either way the frame carries no `stages`, `attempts` or
        // `updatedAt` — building the block from it would render a ladder with no rungs behind it.
        if (!current?.pipeline) {
          // Rule 2: ask for the real row rather than inventing a partial.
          void queryClient.invalidateQueries({ queryKey: key });
          return prev;
        }

        if (!advances(current.pipeline, frame)) return prev;

        const merged = rows.map((r) => (r.hash === frame.hash ? applyFrame(r, frame) : r));
        return { ...res, data: { ...res.data, clips: merged } };
      });
    },
  });
};

// applyFrame paints one frame onto the row it belongs to.
//
// ⚠ It touches only what the frame actually measured. `stages` (the visited ladder), `attempts`
// and `updatedAt` are the GET's to own: the frame carries no value for them, and spreading a
// partial over a full row is how a rendered detail list loses the rungs behind it.
const applyFrame = (clip: IncomingClipDTO, frame: FillerClipEvent): IncomingClipDTO => ({
  ...clip,
  ...(frame.name ? { name: frame.name } : {}),
  // ⚠ The nested block is rebuilt, the clip is spread. `stages`, `attempts` and `updatedAt` live
  // on the block and the frame carries none of them, so replacing the block wholesale would blank
  // the ladder the expanded row renders.
  pipeline: {
    ...clip.pipeline,
    stage: frame.stage,
    status: frame.status,
    progress: frame.progress,
  } as IncomingPipelineDTO,
});

/**
 * advances decides whether this frame describes a state newer than the row already shows.
 *
 * ⚠ **The plan specified "merge only ever advances, using the BE's monotonic `seq`" — and there
 * is no `seq`.** `FillerClipEvent` carries `hash`, `stage`, `status`, `progress`, `disposition`,
 * `reason`, `detail` and `name`; V51b built no sequence number and no timestamp. So ordering is
 * derived from the pipeline's own shape, and the rule is deliberately NARROW:
 *
 * **A transition is always real; only the percentage between transitions is guarded.**
 *
 * ⚠ Strictly advance-only — refusing any frame at or behind the row's ladder position — was
 * rejected, because the pipeline has a legitimate BACKWARD move. The sanctioned re-tag/re-split
 * path (`Rewind`, §10 V51b) resets a suffix of the ladder on purpose, and a strict guard would
 * blank every frame of that re-run until something else forced a refetch. A stale repaint lasts
 * until the next frame; a suppressed re-run looks like the machine has stopped.
 *
 * ⚠ **A STATUS change counts as a transition too, and that is not a detail** — it is what makes
 * the rule survive a retry. A failing stage re-runs as `running` with `Progress` reset to 0 on
 * the same rung (`pipeline.go`'s retry path), so guarding on progress alone would pin the row at
 * "failed at 80%" while the transcode had actually restarted from the beginning. Comparing
 * progress only when stage AND status are both unchanged confines the guard to exactly the
 * intra-stage percentage stream it was written for — which is the same line the emitter already
 * draws: "a status TRANSITION always publishes; only intra-stage percentages are throttled".
 *
 * @param row   the pipeline row currently rendered for this clip
 * @param frame the incoming snapshot
 * @returns whether to merge the frame onto the row
 */
const advances = (row: IncomingPipelineDTO, frame: FillerClipEvent): boolean => {
  if (frame.stage !== row.stage || frame.status !== row.status) return true;
  // ⚠ `>=`, not `>`. Progress is **-1 when the stage cannot measure itself** (the sentinel that
  // renders as an indeterminate spinner), so an unmeasurable rung republishing -1 must not be
  // read as going backwards from -1.
  return frame.progress >= row.progress;
};

export { useClipPipeline };
