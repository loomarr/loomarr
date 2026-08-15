import { formatClipDuration } from "@loomarr/core";
import { Checkbox, Image } from "@/components/ui";
import { cn } from "@/lib";
import { AUDIENCE_LABEL, KIND_LABEL, playsLine } from "../clip-card";
import type { ClipRowProps } from "./clip-row.type";

// ClipRow — the catalog's dense LIST view (V35b, the mock's `catListView`). The mock's column
// spec is `20px 54px 1fr 88px 60px 84px 46px 190px`: checkbox · thumb · name · kind · era ·
// audience · duration · usedLine.
//
// ⚠ **A grid, not a <table>.** An 8-column table at 375px forces horizontal PAGE scroll, which
// `frontend-design.md` forbids; a grid can drop columns per breakpoint, which is what the
// `md:`/`lg:` steps below do. The row is a list item semantically because it carries no
// column headers to associate cells with — the labels are in the values themselves.
//
// ⚠ **No per-row actions, and that is the mock's design rather than an omission.** List view
// is for scanning and bulk-selecting; acting on ONE clip is what the card is for. Adding a
// tag/pin/split button per row would rebuild the card badly in 46px of width.
const ClipRow = ({ clip, selected, onToggleSelect, className }: ClipRowProps) => (
  <div
    className={cn(
      "grid grid-cols-[auto_54px_1fr_auto] items-center gap-3 border-border border-b px-3 py-2 last:border-b-0",
      // The wider steps add the tag columns back. Below `md` they would each be a few
      // characters wide, which is unreadable rather than dense.
      "md:grid-cols-[auto_54px_1fr_88px_60px_auto]",
      "lg:grid-cols-[auto_54px_1fr_88px_60px_84px_auto_190px]",
      selected && "bg-signal/5",
      className,
    )}
  >
    {onToggleSelect ? (
      <Checkbox
        checked={Boolean(selected)}
        onChange={onToggleSelect}
        className="size-4 accent-signal"
        aria-label={`Select ${clip.name}`}
      />
    ) : (
      // Holds the column so rows stay aligned for a member, who gets no checkbox.
      <span aria-hidden />
    )}

    {/* ⚠ Fixed 54×30 box whether or not a frame exists, unlike the card. In a LIST the
        thumbnail is a column: omitting the element for clips without one would ragged the
        name column against its neighbours. An empty box is the honest "no frame" here. */}
    <div className="h-[30px] w-[54px] overflow-hidden rounded-sm bg-static-800">
      {/* ⚠ ONE path since V52 phase 8 — the legacy `/v1/filler/thumb/{hash}` fallback retired with
          its route. A clip whose artwork the adoption job has not reached yet renders the empty
          box above, which is the same honest "no frame" state as a clip that never had one. */}
      {clip.thumbImage && (
        <Image image={clip.thumbImage} alt="" sizes="54px" className="size-full object-cover" />
      )}
    </div>

    {/* ⚠ `min-w-0` is what makes the truncate work. A grid item's default `min-width:auto`
        refuses to shrink below its content, so a long clip name pushes the tag columns off
        the row instead of ellipsising — and the NAME is the one column that must never be the
        thing that gets squeezed out. Caught by reading the baseline image, not by a test:
        every assertion still passed while the name was being crushed. */}
    <span className="min-w-0 truncate text-sm" title={clip.name}>
      {clip.name}
    </span>

    <span className="hidden font-mono text-static-400 text-xs md:inline">{KIND_LABEL[clip.kind]}</span>
    <span className="hidden font-mono text-static-400 text-xs tabular-nums md:inline">
      {clip.era ? `${clip.era}s` : "—"}
    </span>
    <span className="hidden font-mono text-static-400 text-xs lg:inline">
      {(clip.audience && AUDIENCE_LABEL[clip.audience]) || "—"}
    </span>

    <span className="font-mono text-static-400 text-xs tabular-nums">
      {formatClipDuration(clip.durationMs)}
    </span>

    {/* The same three-branch rule the card renders — shared, because the `playsCounted:false`
        branch is a claim about what this install can OBSERVE, and two copies drift. */}
    <span className="hidden truncate font-mono text-2xs text-static-400 lg:inline" title={playsLine(clip)}>
      {playsLine(clip)}
    </span>
  </div>
);

export { ClipRow };
