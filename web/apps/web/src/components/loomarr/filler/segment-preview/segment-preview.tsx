import { clipMediaURL } from "@loomarr/core/clip-thumb";
import { formatMmSs } from "@loomarr/core/format";
import { MediaTile, Stack, Text, VisuallyHidden } from "@loomarr/design-system";
import { useRef, useState } from "react";
import { VideoPlayer } from "@/components/loomarr/player/video-player";
import { ServedImage } from "@/components/loomarr/served-image";
import type { SegmentPreviewProps } from "./segment-preview.type";

// SegmentPreview — one proposed cut, played from the parent recording (§10 V54/V70). It remains
// a lazy tile where used alone and renders only the bounded player inside split review's Sheet.
//
// ⚠ **Why it exists.** The split-review page asks an operator whether a cut at 04:17 is right, and
// name, time, Merge, Drop and Confirm controls give them nothing to see or hear.
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
  artwork,
  open,
  onOpenChange,
  autoPlay,
  embedded,
}: SegmentPreviewProps) => {
  const buttonRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const sourceKey = `${clipHash}:${startMs}:${endMs}`;
  const [failedSource, setFailedSource] = useState<string>();
  const failed = failedSource === sourceKey;

  const span = endMs - startMs;
  const panelId = `seg-preview-${position}`;
  const verbId = `seg-preview-verb-${position}`;
  const unavailableId = `seg-preview-na-${position}`;
  const spanId = `seg-preview-span-${position}`;

  const player = failed ? (
    <Stack
      align="center"
      aspectRatio={16 / 9}
      backgroundColor="$surfaceRaised"
      borderRadius={6}
      direction="row"
      justify="center"
      padding={24}
      role="alert"
    >
      <Text as="span" textAlign="center" textRole="compact" tone="secondary">
        This preview couldn’t be played. The original recording may be unavailable.
      </Text>
    </Stack>
  ) : (
    <VideoPlayer
      src={clipMediaURL(clipHash)}
      startAt={startMs / 1000}
      endAt={endMs / 1000}
      autoPlay={autoPlay}
    />
  );

  if (embedded) {
    return (
      <div style={{ width: "100%" }} onErrorCapture={() => setFailedSource(sourceKey)}>
        {player}
      </div>
    );
  }

  return (
    // Escape is scoped to this subtree on purpose: a document-level handler in a 52-row list
    // would collapse whichever preview it reached first. The interactive controls inside are the
    // tile and the player's own — this wrapper only listens.
    // biome-ignore lint/a11y/noStaticElementInteractions: keyboard-only listener, see above.
    <div
      style={{ display: "contents" }}
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
      <MediaTile
        ref={buttonRef}
        // The mock's tooltip, verbatim (`loomarr-prototype-desktop-v2.dc.html:2209`).
        title="Preview this segment"
        // ⚠ The name is composed from a fixed verb plus the VISIBLE "#N" marker, never from the
        // adjacent Name <input>: `aria-labelledby` on an input takes its VALUE, so the button
        // would be renamed on every keystroke. Anchoring to something on screen is also what the
        // compact-control rule asks for — a hidden-only name drifts from what the operator sees.
        // The tile's own visible words ("Preview unavailable", the timecode badge) belong in the
        // name too, in reading order (WCAG 2.5.3). The badge stays aria-hidden so it is read once,
        // as part of the name; labelledby still resolves hidden elements.
        aria-labelledby={[verbId, labelledBy, !artwork && unavailableId, span > 0 && spanId]
          .filter(Boolean)
          .join(" ")}
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        // Open is a pressed state, so it is neutral (#1909); the amber outline is focus.
        current={open}
        height={64}
        onPress={() => {
          if (!open) setFailedSource(undefined);
          onOpenChange(!open);
        }}
        radius="medium"
        width={112}
      >
        <VisuallyHidden as="span">
          <span id={verbId}>Preview segment</span>
        </VisuallyHidden>
        {artwork ? (
          <ServedImage image={artwork} alt="" sizes="112px" />
        ) : (
          <Stack paddingHorizontal={8} render="span">
            <Text as="span" id={unavailableId} textAlign="center" textRole="guideCell" tone="secondary">
              Preview unavailable
            </Text>
          </Stack>
        )}
        <Stack
          align="center"
          aria-hidden
          bottom={0}
          direction="row"
          justify="center"
          left={0}
          position="absolute"
          render="span"
          right={0}
          style={{ backgroundColor: "color-mix(in srgb, var(--color-static-950) 25%, transparent)" }}
          top={0}
        >
          <Text as="span" style={{ color: "var(--color-static-0)" }} textRole="compact">
            ▶
          </Text>
        </Stack>
        {/* ⚠ `formatMmSs` ("00:30"), NOT `formatClipDuration` ("30s"). The row already renders the
            span as "30s", and a second "30s" inside the same region makes the editor's existing
            `getByText("30s")` match two elements and throw. It also reads better: the row's is a
            validation readout, this is a timecode badge on a thumbnail. */}
        {span > 0 && (
          <Stack
            borderRadius={2}
            bottom={4}
            paddingHorizontal={4}
            position="absolute"
            render="span"
            right={4}
            style={{ backgroundColor: "color-mix(in srgb, var(--color-static-950) 85%, transparent)" }}
          >
            <Text as="span" id={spanId} aria-hidden textRole="microBadge">
              {formatMmSs(span)}
            </Text>
          </Stack>
        )}
      </MediaTile>

      {open && (
        <div
          ref={panelRef}
          id={panelId}
          style={{ marginTop: 8, maxWidth: 448, width: "100%" }}
          onErrorCapture={() => setFailedSource(sourceKey)}
        >
          {/* No `title`: the row's Name field names this segment eight pixels away, and the player
              does not repeat a heading that already says what it is. */}
          {player}
        </div>
      )}
    </div>
  );
};

export { SegmentPreview };
