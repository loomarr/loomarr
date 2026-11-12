import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import { formatClipDuration, formatRelative } from "@loomarr/core/format";
import { Icon, icons, Stack, Tooltip } from "@loomarr/design-system";
import { CardAction, CardStatus, ClipSelect, PictureCard } from "@loomarr/ui";
import { useState } from "react";
import { ArtworkFallback } from "@/components/loomarr/artwork-fallback";
import { ServedImage } from "@/components/loomarr/served-image";
import { cn } from "@/lib/utils";
import type { ClipCardProps } from "./clip-card.type";

// ClipCard — a filler clip with its match tags (§3, §10): role/era/audience/category
// chips + a mono, sub-minute-aware duration. tagged clips are pod-ready; untagged
// clips flag caution with a "Tag" action.
const ROLE_LABEL: Record<ClipDTO["role"], string> = {
  unclassified: "Unclassified",
  commercial: "Commercial",
  bumper: "Bumper",
  station_id: "Station ID",
  psa: "PSA",
  trailer: "Trailer",
  interstitial: "Interstitial",
  political: "Political ad",
  news: "News & weather",
};

// Keyed loosely because ClipDTO's audience includes "" (unset); the guard below skips it.
const AUDIENCE_LABEL: Record<string, string> = {
  kids: "Kids",
  family: "Family",
  general: "General",
  late_night: "Late night",
};

// eraLabel renders an era for display. ⚠ The tagger grounds a LITERAL YEAR from the clip's text
// (§8), so `era` is often a specific year (1996), NOT a decade — and "1996s" is nonsense. Append the
// decade "s" ONLY when the value is a decade boundary (divisible by 10); a specific year renders
// as-is. 1990 → "1990s"; 1996 → "1996". 0/undefined → "" (the caller shows an "era" placeholder).
const eraLabel = (era: number | undefined): string => {
  if (!era) return "";
  return era % 10 === 0 ? `${era}s` : String(era);
};
// ⚠ No hardcoded CATEGORIES list here (§10 V45a). `category` is a DERIVED shadow of the taxonomy
// tags, not a directly-editable free string — there is nothing to cycle. Tags are shown as read-only
// badges and edited in the tag dialog, which serves the real vocabulary. (A list here would also break
// the no-hardcode rule: operator-editable data hardcoded on the FE.)

// How much this clip has actually AIRED — the mock's `usedLine`.
//
// ⚠ The verb is "aired", not "played", deliberately: "played" reads as "have I watched this", and
// the Play button on the card visibly does not move the number. The count
// is an AIRINGS counter — incremented only when internal playout actually broadcasts the clip
// (store.RecordClipPlay, "written from playout only"), NEVER by watching the preview. "Aired" is what
// the number measures, and it disambiguates from the Play button (which is watching, not airing).
//
// ⚠ Exported because the LIST row (V35b) shows the same line, and this is a three-branch rule where
// the wrong branch states a falsehood. `playsCounted:false` is NOT zero airings: it means this install
// cannot OBSERVE airings (Tunarr-backed playout owns the stream), so "Never aired" there would be a
// lie the DTO's own comment warns against. One rule, shared, so the two renderers cannot drift.
const playsLine = (clip: ClipDTO): string => {
  if (!clip.playsCounted) return "Airings aren't counted on this setup";
  if (clip.playCount === 0) return "Never aired";
  const airings = `${clip.playCount} ${clip.playCount === 1 ? "airing" : "airings"}`;
  return clip.lastPlayedAt ? `${airings} · last ${formatRelative(clip.lastPlayedAt)}` : airings;
};

// ClipFrame — the thumbnail, its overlays, and the hover preview (V39).
//
// Its own component because it is the only stateful part of an otherwise pure card, and inlining
// a `useState` would make every card in a 200-card grid re-render on any one card's hover.
//
// ⚠ **The animation is a generated ASSET, not the clip streamed on hover.** Playing the source on
// hover is the cheaper build and the wrong one at catalog scale: a grid would open a range
// request per hovered card against files tens of megabytes each, to show six seconds. The webp is
// ~150KB and already the right length. See internal/filler/preview.go.
const ClipFrame = ({
  clip,
  selected,
  onToggleSelect,
  onPlay,
}: {
  clip: ClipDTO;
  selected?: boolean;
  onToggleSelect?: () => void;
  onPlay?: () => void;
}) => {
  const { thumbImage } = clip;
  // ⚠ **Unmounted on leave, and that is what makes the animation RESTART.** An animated WebP
  // begins at frame 0 when it decodes and then runs on its own — the browser keeps it going while
  // the element lives, whether or not anyone is looking. Keeping it mounted and merely hiding it
  // would make the second hover pick the loop up wherever it happens to be, usually mid-advert.
  // Remounting forces a fresh decode, so every hover starts from the beginning. (Maintainer,
  // 2026-08-03.)
  //
  // The re-fetch this implies is cheap: the asset carries `Cache-Control: private, max-age=3600`,
  // so a second hover is a memory-cache hit rather than a network round trip.
  //
  // ⚠ Still nothing at all before the first hover. A catalog is hundreds of cards, and mounting
  // every preview up front would pull the whole grid's worth of animation immediately — precisely
  // what the still exists to avoid.
  const [hovered, setHovered] = useState(false);
  // ⚠ Keys on `hoverImage` (V52 phase 8): "is there an animation to show?" is answered by the
  // image record, not by a `preview` path.
  const showPreview = hovered && Boolean(clip.hoverImage);

  // ⚠ These handlers are DECORATIVE: they reveal a preview and do nothing else. The real control
  // is the play button inside, which is already focusable and already flips this same state via
  // `onFocus` — so a keyboard user reaches everything a mouse user does.
  //
  // ⚠ Do NOT "fix" the lint by adding `role`/`tabIndex` here. That would add a tab stop that
  // activates nothing, and ARIA copied onto something that merely looks interactive is how this
  // repo has already earned a CRITICAL axe violation (see nav-tabs' aria-controls note).
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: decorative hover only, see above
    <div
      // Fills the picture card's media box, which sets the ratio and crops to it.
      className="group relative size-full overflow-hidden"
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      // Focus counts as hover so a keyboard user tabbing to the play button sees the same preview
      // a mouse user does. `onBlur` mirrors it for the same restart reason.
      onFocus={() => setHovered(true)}
      onBlur={() => setHovered(false)}
    >
      {/* ⚠ ONE path (V52 phase 8), with no thumbnail-route fallback: a clip's artwork is an
          image-service image (§22).

          Empty alt, deliberately: the clip's name is the very next element, so a description here
          would have a screen reader announce the same clip twice. The frame is decoration for a
          label that is already present.

          No `priority`: a catalog is hundreds of cards, and <ServedImage> defaults to lazy +
          async, which is exactly right below the fold. */}
      {/* A clip with no extracted frame still fills the card's picture slot with the neutral
          code-drawn fallback, as every card with a picture does (#1817 Q12). */}
      {thumbImage ? (
        <ServedImage image={thumbImage} alt="" sizes="(max-width: 640px) 45vw, 220px" />
      ) : (
        <ArtworkFallback />
      )}
      {/* The animation, stacked ON the still rather than replacing its src. Swapping the src
          would blank the box for as long as the webp took to arrive — a flash of empty card on
          every hover — and would lose the still if the preview 404'd. Layering means the still is
          simply covered once there is something to cover it with. */}
      {showPreview && clip.hoverImage && (
        // ⚠ `fallback={null}` — render NOTHING on failure, revealing the still beneath. This is
        // the one caller that wants no visible failure state at all: a colour block (the
        // primitive's default) would be a visible fault over the frame where the honest answer
        // is "this clip has no preview", which is the common case on any install that has not
        // re-synced.
        //
        // The animation is ONE rendition and skips the ladder (§22 `animated`), so `sizes` is
        // nominal here — it is the still beneath that does the responsive work.
        <span className="absolute inset-0">
          <ServedImage
            image={clip.hoverImage}
            alt=""
            sizes="(max-width: 640px) 45vw, 220px"
            fallback={null}
          />
        </span>
      )}

      {/* Overlays (V35b, the mock's card): duration bottom-right, quality top-right, select
          top-left. ⚠ Each sits on a scrim (`bg-static-900/80`) rather than directly on the frame
          — a thumbnail is arbitrary video, so text with no backing plate has NO guaranteed
          contrast against it. That is a legibility rule, not a style choice. */}
      {clip.quality && (
        // ⚠ `role="img"` is load-bearing, not decoration. `aria-label` on a BARE span is ignored —
        // ARIA only permits naming on an element that has a role, and the chip-row version below
        // gets one from Badge. Without it the label silently does nothing and "1080P" is announced
        // as letter-spaced shouting, which is the whole reason the label exists.
        <span
          role="img"
          className="bg-static-900/80 text-2xs text-static-100 absolute top-1.5 right-1.5 rounded px-1.5 py-0.5 font-mono"
          title="Resolution, from the clip's video height"
          aria-label={`Resolution ${clip.quality}`}
        >
          {clip.quality}
        </span>
      )}
      {onToggleSelect && (
        <div className="absolute top-0.5 left-0.5 z-10">
          <ClipSelect
            clipName={clip.name}
            selected={Boolean(selected)}
            onSelectedChange={() => onToggleSelect()}
          />
        </div>
      )}

      {/* The play button — the mock's 58px amber disc (line 982 of the desktop prototype),
          scaled to this smaller frame. Centred over the preview.

          ⚠ A real <button>, not a click handler on the frame. The whole card is not clickable
          (its chips and actions are), so a bare div-with-onClick here would be an invisible
          target with no keyboard route and no accessible name.

          ⚠ Always in the DOM, revealed on hover/focus by opacity — NOT conditionally rendered.
          A button that only exists on hover cannot be reached by Tab, so a keyboard user could
          never play a clip. `group-focus-within` is what makes tabbing to it reveal it. */}
      {onPlay && thumbImage && (
        <Stack
          align="center"
          className="pointer-events-none absolute inset-0"
          direction="row"
          justify="center"
        >
          {/* A plain <button>, not a DS Action: the target is the whole preview frame with no
              control box of its own, which no Action layout draws. So the pointer cursor (which
              Tailwind v4's Preflight stopped giving <button>) and the focus ring are set here.
              Only the disc is visible. */}
          <button
            type="button"
            onClick={onPlay}
            // The name says WHICH clip: a grid of buttons all called "Play" is unusable by voice
            // control and meaningless in a screen reader's element list.
            aria-label={`Preview ${clip.name}`}
            className={cn(
              "text-static-950 pointer-events-auto flex h-full w-full cursor-pointer items-center justify-center",
              // Visible focus ring against arbitrary video underneath.
              "focus-visible:ring-static-100 outline-none focus-visible:ring-2 focus-visible:ring-inset",
            )}
          >
            {/* ⚠ Nudged right by a pixel: a triangle's optical centre is left of its bounding
                box, so a mathematically centred play glyph reads as sitting too far left. The
                mock does the same thing with `padding-left:4px`. */}
            <Stack
              align="center"
              className="bg-signal/90 size-11 rounded-full opacity-0 shadow-lg transition-opacity group-focus-within:opacity-100 group-hover:opacity-100"
              direction="row"
              justify="center"
              render="span"
            >
              <Stack className="ml-0.5" direction="row" render="span">
                <Icon decorative glyph={icons.play} size="default" tone="inherit" />
              </Stack>
            </Stack>
          </button>
        </Stack>
      )}
    </div>
  );
};

// ClipCard is a picture card (#2765): the frame at a square ratio with its length in the corner, the
// name, ONE metadata line (what is wrong with it, kind, era, audience, how much it has aired) and ONE
// action. Brand, category and the other tags live in the details panel.
const ClipCard = ({ clip, onOpen, onSplit, splitPending, selected, onToggleSelect }: ClipCardProps) => {
  // Ready is the default and wears no status (#1817, decided on #1908): a healthy library of a
  // thousand clips shouldn't carry a thousand warnings. Only a clip that is NOT ready names its
  // state.
  const exception = clip.held ? "Preparing" : clip.isComposite ? "Recording" : undefined;
  // One exception per clip: a clip still preparing or a recording is not yet the thing to tag.
  // Only the server's `needsDetails`, never kind + tagged, so the status agrees with the filter.
  const needsDetails = !exception && clip.needsDetails;
  const status = exception ? (
    <CardStatus key="status" label={exception} tone={clip.held ? "pending" : "progress"} />
  ) : needsDetails ? (
    <CardStatus key="status" label="Needs details" tone="warn" />
  ) : null;
  const meta = [
    ...(status ? [status] : []),
    ...(clip.role !== "unclassified" ? [ROLE_LABEL[clip.role]] : []),
    ...(clip.era ? [eraLabel(clip.era)] : []),
    ...(clip.audience ? [AUDIENCE_LABEL[clip.audience] ?? clip.audience] : []),
    playsLine(clip),
  ];
  // A composite offers its split as the one action, so its name opens the details instead.
  const nameOpens = onSplit && onOpen;
  return (
    <div className={cn("rounded-lg", selected && "ring-foreground ring-1")}>
      <PictureCard
        action={
          onSplit ? (
            <Tooltip content="Detect commercials inside this compilation and review the cuts">
              <CardAction disabled={splitPending} icon="split" onPress={onSplit}>
                {splitPending ? "Splitting…" : "Split into clips"}
              </CardAction>
            </Tooltip>
          ) : onOpen ? (
            <CardAction
              accessibilityLabel={`View details for ${clip.name}`}
              attention={needsDetails}
              onPress={onOpen}
            >
              Details
            </CardAction>
          ) : undefined
        }
        corner={formatClipDuration(clip.durationMs)}
        media={<ClipFrame clip={clip} selected={selected} onToggleSelect={onToggleSelect} onPlay={onOpen} />}
        meta={meta}
        orientation="top"
        ratio="1/1"
        title={
          nameOpens ? (
            <button
              type="button"
              onClick={onOpen}
              aria-label={`View details for ${clip.name}`}
              className="hover:text-signal focus-visible:ring-ring rounded-sm text-left focus-visible:ring-2 focus-visible:outline-none"
            >
              {clip.name}
            </button>
          ) : (
            clip.name
          )
        }
      />
    </div>
  );
};

export { AUDIENCE_LABEL, ClipCard, playsLine, ROLE_LABEL };
