import { formatClipDuration } from "@loomarr/core/format";
import { ClipSelect, NeedsDetailsChip } from "@loomarr/ui";
import { Image } from "@/components/ui/image";
import { cn } from "@/lib/utils";
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
// Titles and thumbnails open the same inspection panel as Grid, without extra action rows.
const ClipRow = ({ clip, selected, onToggleSelect, onOpen, className }: ClipRowProps) => (
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
      <ClipSelect
        clipName={clip.name}
        selected={Boolean(selected)}
        onSelectedChange={() => onToggleSelect()}
      />
    ) : (
      // Holds the column so rows stay aligned for a member, who gets no checkbox.
      <span aria-hidden />
    )}

    {/* ⚠ Fixed 54×30 box whether or not a frame exists, unlike the card. In a LIST the
        thumbnail is a column: omitting the element for clips without one would ragged the
        name column against its neighbours. An empty box is the honest "no frame" here. */}
    <button
      type="button"
      disabled={!onOpen}
      onClick={onOpen}
      aria-label={`Preview ${clip.name}`}
      className="h-[30px] w-[54px] overflow-hidden rounded-sm bg-static-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      {/* ⚠ ONE path since V52 phase 8 — the legacy `/v1/filler/thumb/{hash}` fallback retired with
          its route. A clip whose artwork the adoption job has not reached yet renders the empty
          box above, which is the same honest "no frame" state as a clip that never had one. */}
      {clip.thumbImage && (
        <Image image={clip.thumbImage} alt="" sizes="54px" className="size-full object-cover" />
      )}
    </button>

    {/* ⚠ `min-w-0` is what makes the truncate work. A grid item's default `min-width:auto`
        refuses to shrink below its content, so a long clip name pushes the tag columns off
        the row instead of ellipsising — and the NAME is the one column that must never be the
        thing that gets squeezed out. Caught by reading the baseline image, not by a test:
        every assertion still passed while the name was being crushed. */}
    <div className="min-w-0">
      {onOpen ? (
        <button
          type="button"
          onClick={onOpen}
          aria-label={`View details for ${clip.name}`}
          // `-my-0.5 py-0.5`: a 24 px tap area for a 20 px line, drawn where it was (#1785). The
          // shared `data-touch-target` box can't be used here: `truncate` clips it.
          className="-my-0.5 block w-full truncate rounded-sm py-0.5 text-left text-sm hover:text-signal focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          title={clip.name}
        >
          {clip.name}
        </button>
      ) : (
        <p className="truncate text-sm" title={clip.name}>
          {clip.name}
        </p>
      )}
      {clip.brand ? (
        <p className="truncate text-muted-foreground text-xs" title={`Brand: ${clip.brand}`}>
          {clip.brand}
        </p>
      ) : null}
      {/* Phone only, as the mock draws it: below `md` the tag columns are gone, so the row has no
          other way to say this commercial is missing one. */}
      {clip.needsDetails && !clip.held && !clip.isComposite ? (
        <div className="mt-0.5 flex md:hidden">
          <NeedsDetailsChip />
        </div>
      ) : null}
    </div>

    <span className="hidden font-mono text-static-400 text-xs md:inline">
      {clip.kind === "unclassified" ? "" : KIND_LABEL[clip.kind]}
    </span>
    <span className="hidden font-mono text-static-400 text-xs tabular-nums md:inline">
      {clip.era ? (clip.era % 10 === 0 ? `${clip.era}s` : String(clip.era)) : "—"}
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
