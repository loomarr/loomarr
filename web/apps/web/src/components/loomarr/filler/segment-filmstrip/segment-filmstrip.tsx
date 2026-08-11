import { formatMmSs } from "@loomarr/core";
import { cn } from "@/lib";
import type { SegmentFilmstripProps } from "./segment-filmstrip.type";

// SegmentFilmstrip — the reel's detected clips as one time-scaled bar (the v2 mock's `rl.strip`).
//
// ⚠ **Block width is PROPORTIONAL TO DURATION, and that is the whole point.** The mock computes
// `flex: round(seg.duration / total * 1000)`, so a 45-second advert is visibly wider than a
// 3-second sting. Equal-width blocks would draw the same picture for a well-split reel and a
// badly-split one — which is precisely the judgement the operator opens this to make. A strip
// that cannot show a bad split is decoration.
//
// ⚠ Rendered from the editor's DRAFT, not from the proposal. Merging two segments has to widen
// one block and remove another as it happens; reading the server's copy would leave the picture
// describing a split the operator already changed.

// minBlockPercent keeps a very short segment clickable.
//
// ⚠ Without it a 0.5s sting in a 20-minute reel computes to well under a pixel — present in the
// DOM, impossible to hit, and invisible. The distortion is deliberate and bounded: it is better
// for the shortest blocks to read slightly wide than for them to be unreachable.
const minBlockPercent = 0.6;

const SegmentFilmstrip = ({ segments, activeKey, onFocus, className }: SegmentFilmstripProps) => {
  const spans = segments.map((s) => ({ ...s, durationMs: Math.max(0, s.endMs - s.startMs) }));
  const total = spans.reduce((sum, s) => sum + s.durationMs, 0);

  // ⚠ An empty or zero-length reel renders NOTHING rather than a bar of NaN-width blocks. A
  // proposal whose segments all collapsed is a real state (every one dropped in the editor), and
  // dividing by its total is how a timeline becomes a row of invisible artefacts.
  if (spans.length === 0 || total <= 0) return null;

  const reelEnd = Math.max(...spans.map((s) => s.endMs));

  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      {/* ⚠ `role="group"` is wrong here and `<ul>` is right: this is a LIST of segments, and a
          screen reader announcing "list, 12 items" tells the operator how many clips the
          detector found before they touch anything. */}
      <ul className="flex h-6 list-none gap-0.5" aria-label="Detected clips, in order">
        {spans.map((s) => {
          const pct = Math.max(minBlockPercent, (s.durationMs / total) * 100);
          const label = `${formatMmSs(s.startMs)} · ${s.name || "unnamed"}`;
          return (
            <li key={s.key} style={{ flex: `${pct} 1 0%` }} className="min-w-[3px]">
              <button
                type="button"
                title={label}
                // ⚠ The accessible name carries the TIMECODE as well as the name. Blocks are
                // visually distinguished by width and position, neither of which survives into
                // a screen reader — without the timecode every unnamed block reads "unnamed".
                aria-label={label}
                aria-current={activeKey === s.key ? "true" : undefined}
                onClick={() => onFocus?.(s.key)}
                className={cn(
                  "h-full w-full rounded-sm border-none p-0 transition-opacity",
                  // The mock's muted default with a hover to full — the strip is a glance, not
                  // a focal point, until the operator reaches for it.
                  "opacity-75 hover:opacity-100",
                  // ⚠ A visible focus ring. The mock's blocks have none, and keyboard users
                  // would have no idea which block they were on — the same class of gap as the
                  // mouse-only palette the a11y sweep caught.
                  "focus-visible:opacity-100 focus-visible:outline focus-visible:outline-2 focus-visible:outline-signal focus-visible:outline-offset-1",
                  // ⚠ `static-500`, not `static-600`. The first draft used 600 and the visual
                  // snapshot showed the result immediately: a 14-minute unsplittable remainder
                  // rendered so close to the background that the strip appeared to be MISSING
                  // its largest block — in the very story written to demonstrate that case.
                  // Eleven passing unit tests could not see it; one screenshot could. Matches
                  // `guide-detail-card` and `on-air-indicator`, which use 500 for the same
                  // muted-but-visible job.
                  s.unsplittable ? "bg-static-500" : "bg-signal",
                  activeKey === s.key && "opacity-100 ring-1 ring-signal-300",
                )}
              />
            </li>
          );
        })}
      </ul>
      <div className="flex justify-between font-mono text-[10px] text-muted-foreground">
        <span>{formatMmSs(0)}</span>
        {/* ⚠ **"jump to its row", NOT "preview" (V54 A7).** This read "click to preview" for as
            long as the strip has existed, and clicking has never previewed anything: `onFocus`
            sets the editor's `focusedKey`, which scrolls that segment's row into view and rings
            it. Confirmed live — the page contains zero `<video>`, `<canvas>` and `<img>`
            elements. A caption is a promise about behaviour, and an operator who clicks expecting
            to see the clip concludes the feature is broken rather than absent.

            The preview itself is Phase C. When it lands, this wording changes WITH it — the
            "does not promise a preview it cannot deliver" test below is what ties the two
            together, and it relaxes on its own once a media element is rendered. */}
        <span>every block is one detected clip — click to jump to its row</span>
        <span>{formatMmSs(reelEnd)}</span>
      </div>
    </div>
  );
};

export { SegmentFilmstrip };
