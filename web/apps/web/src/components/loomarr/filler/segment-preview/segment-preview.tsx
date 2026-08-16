import { clipMediaURL } from "@loomarr/core/clip-thumb";
import { formatMmSs } from "@loomarr/core/format";
import { useRef } from "react";
import { VideoPlayer } from "@/components/ui/video-player";
import { cn } from "@/lib/utils";
import type { SegmentPreviewProps } from "./segment-preview.type";

// SegmentPreview — one proposed cut, played in place (§10 V54).
//
// ⚠ **The gap this closes.** The split-review page asked an operator whether a cut at 04:17 was
// right and gave them nothing to see or hear: measured 2026-08-12 on a 52-segment reel, the page's
// entire toolkit was a name field, two mm:ss fields, Merge, Drop and Confirm. V54 A7 was honest
// about it — it deleted the mock's "click to preview" caption because clicking had never previewed
// anything — but the affordance stayed unbuilt.
//
// ⚠ **It plays the PARENT, not the segment.** A proposed cut has no bytes until `confirm` writes
// them, so the preview is a byte-range window of the composite it was detected in. That is the
// operational reason §10 V45's keep-the-parent rule matters to an operator and not only to lineage:
// without the retained reel there is nothing to play.
//
// ⚠ Collapsing UNMOUNTS the player. Hiding it with CSS would leave a range request open against a
// 20-minute file for every row the operator has ever clicked.
const SegmentPreview = ({
  clipHash,
  startMs,
  endMs,
  position,
  labelledBy,
  open,
  onOpenChange,
  autoPlay,
  className,
}: SegmentPreviewProps) => {
  const buttonRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  const span = endMs - startMs;
  const panelId = `seg-preview-${position}`;
  const verbId = `seg-preview-verb-${position}`;

  return (
    // Escape is scoped to this subtree on purpose: a document-level handler in a 52-row list
    // would collapse whichever preview it reached first. The interactive controls inside are the
    // tile and the player's own — this wrapper only listens.
    // biome-ignore lint/a11y/noStaticElementInteractions: keyboard-only listener, see above.
    <div
      className={cn("contents", className)}
      onKeyDown={(e) => {
        // ⚠ On the WRAPPER, not the panel. The tile and the panel are siblings, so a handler on
        // the panel never sees a keypress made while the tile itself has focus — which is the
        // common case, since the tile is what the operator just clicked.
        if (!open || e.key !== "Escape") return;
        e.stopPropagation();
        onOpenChange(false);
        // ⚠ CONDITIONAL. Under one-open-at-a-time, opening row 8 collapses row 7 — and an
        // unconditional refocus would then yank focus back to row 7's tile, away from the row the
        // operator just asked for.
        if (panelRef.current?.contains(document.activeElement)) buttonRef.current?.focus();
      }}
    >
      <button
        ref={buttonRef}
        type="button"
        // The mock's tooltip, verbatim (`loomarr-prototype-desktop-v2.dc.html:2209`).
        title="Preview this segment"
        // ⚠ The name is composed from a fixed verb plus the VISIBLE "#N" marker, never from the
        // adjacent Name <input>: `aria-labelledby` on an input takes its VALUE, so the button
        // would be renamed on every keystroke. Anchoring to something on screen is also what the
        // compact-control rule asks for — an sr-only-only name drifts from what the operator sees.
        aria-labelledby={`${verbId} ${labelledBy}`}
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        onClick={() => onOpenChange(!open)}
        className={cn(
          // The mock's geometry (`:2208-2211`): 84×47, the ▶ centred, a duration badge bottom-right.
          "relative h-[47px] w-21 shrink-0 cursor-pointer rounded-[4px] border-none p-0",
          "flex items-center justify-center",
          // ⚠ A visible focus ring the mock does not have — the same gap the filmstrip's blocks
          // already record fixing. Keyboard users would otherwise have no idea which tile they are on.
          "focus-visible:outline focus-visible:outline-2 focus-visible:outline-signal focus-visible:outline-offset-1",
          // ⚠ Token-sourced stripes rather than the mock's free-running `hsl({{hue}} 40% 22%)`.
          // A raw hue puts colours in the product that the contrast generator has never checked;
          // `channel-ident` made the same call for the same reason. Geometry is the mock's.
          "bg-[repeating-linear-gradient(135deg,var(--color-static-800)_0_5px,var(--color-static-900)_5px_10px)]",
          open && "ring-1 ring-signal",
        )}
      >
        <span id={verbId} className="sr-only">
          Preview segment
        </span>
        <span aria-hidden className="text-[13px] text-white/72">
          ▶
        </span>
        {/* ⚠ `formatMmSs` ("00:30"), NOT `formatClipDuration` ("30s"). The row already renders the
            span as "30s", and a second "30s" inside the same region makes the editor's existing
            `getByText("30s")` match two elements and throw. It also reads better: the row's is a
            validation readout, this is a timecode badge on a thumbnail. */}
        {span > 0 && (
          <span
            aria-hidden
            className="absolute right-1 bottom-[3px] rounded-[2px] bg-static-950/80 px-1 font-mono text-[9px] text-static-100"
          >
            {formatMmSs(span)}
          </span>
        )}
      </button>

      {open && (
        <div ref={panelRef} id={panelId} className="mt-2 w-full max-w-md">
          {/* No `title`: the row's Name field names this segment eight pixels away, and the player
              does not repeat a heading that already says what it is. */}
          <VideoPlayer
            src={clipMediaURL(clipHash)}
            startAt={startMs / 1000}
            endAt={endMs / 1000}
            autoPlay={autoPlay}
          />
        </div>
      )}
    </div>
  );
};

export { SegmentPreview };
