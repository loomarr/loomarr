import { formatClipDuration } from "@loomarr/core/format";
import { Grid, InlineAction, MediaTile, Text } from "@loomarr/design-system";
import { ClipSelect, NeedsDetailsChip } from "@loomarr/ui";
import { ServedImage } from "@/components/loomarr/served-image";
import { cn } from "@/lib/utils";
import { AUDIENCE_LABEL, playsLine, ROLE_LABEL } from "../clip-card";
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
  <Grid
    align="center"
    className={cn(
      "border-border border-b px-3 py-2 last:border-b-0",
      // Selection is neutral (#1909): amber is for focus and the primary action.
      selected && "bg-static-800",
      className,
    )}
    // The wider steps add the tag columns back. Below `md` they would each be a few characters
    // wide, which is unreadable rather than dense.
    columns={{
      base: ["auto", "54px", "1fr", "auto"],
      lg: ["auto", "54px", "1fr", "88px", "60px", "84px", "auto", "190px"],
      md: ["auto", "54px", "1fr", "88px", "60px", "auto"],
    }}
    gap="3"
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
    <MediaTile
      bordered={false}
      disabled={!onOpen}
      height={30}
      label={`Preview ${clip.name}`}
      onPress={onOpen}
      width={54}
    >
      {/* ⚠ ONE path (V52 phase 8): the artwork is an image-service image, with no thumbnail-route
          fallback. A clip whose artwork the adoption job has not reached yet renders the empty
          box above, which is the same honest "no frame" state as a clip that never had one. */}
      {clip.thumbImage && <ServedImage image={clip.thumbImage} alt="" sizes="54px" />}
    </MediaTile>

    {/* ⚠ `min-w-0` is what makes the truncate work. A grid item's default `min-width:auto`
        refuses to shrink below its content, so a long clip name pushes the tag columns off
        the row instead of ellipsising — and the NAME is the one column that must never be the
        thing that gets squeezed out. Caught by reading the baseline image, not by a test:
        every assertion still passed while the name was being crushed. */}
    <div className="min-w-0">
      {onOpen ? (
        // The name is the DS's truncating text action: its 24 px tap area is its own padding (#1785),
        // since the usual `::after` target is clipped by the ellipsis.
        <Text as="div" textRole="compact" tone="primary">
          <InlineAction
            aria-label={`View details for ${clip.name}`}
            onPress={onOpen}
            title={clip.name}
            truncate
          >
            {clip.name}
          </InlineAction>
        </Text>
      ) : (
        <div title={clip.name}>
          <Text as="p" numberOfLines={1} textRole="compact" tone="primary">
            {clip.name}
          </Text>
        </div>
      )}
      {clip.brand ? (
        <div title={`Brand: ${clip.brand}`}>
          <Text as="p" numberOfLines={1} textRole="hint" tone="secondary">
            {clip.brand}
          </Text>
        </div>
      ) : null}
      {/* Phone only, as the mock draws it: below `md` the tag columns are gone, so the row has no
          other way to say this commercial is missing one. */}
      {clip.needsDetails && !clip.held && !clip.isComposite ? (
        <div className="mt-0.5 flex md:hidden">
          <NeedsDetailsChip />
        </div>
      ) : null}
    </div>

    <Text as="span" className="hidden md:inline" textRole="mono">
      {clip.role === "unclassified" ? "" : ROLE_LABEL[clip.role]}
    </Text>
    <Text as="span" className="hidden md:inline" fontVariant={["tabular-nums"]} textRole="mono">
      {clip.era ? (clip.era % 10 === 0 ? `${clip.era}s` : String(clip.era)) : "—"}
    </Text>
    <Text as="span" className="hidden lg:inline" textRole="mono">
      {(clip.audience && AUDIENCE_LABEL[clip.audience]) || "—"}
    </Text>

    <Text as="span" fontVariant={["tabular-nums"]} textRole="mono">
      {formatClipDuration(clip.durationMs)}
    </Text>

    {/* The same three-branch rule the card renders — shared, because the `playsCounted:false`
        branch is a claim about what this install can OBSERVE, and two copies drift. */}
    <div className="hidden lg:block" title={playsLine(clip)}>
      <Text as="div" numberOfLines={1} textRole="mono" tone="secondary">
        {playsLine(clip)}
      </Text>
    </div>
  </Grid>
);

export { ClipRow };
