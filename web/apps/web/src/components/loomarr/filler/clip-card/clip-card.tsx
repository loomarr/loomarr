import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import { formatClipDuration, formatRelative } from "@loomarr/core/format";
import { Pin, Play, Scissors, Tag } from "lucide-react";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Image } from "@/components/ui/image";
import { cn } from "@/lib/utils";
import type { ClipCardProps } from "./clip-card.type";

// ClipCard — a filler clip with its match tags (§3, §10): kind/era/audience/category
// chips + a mono, sub-minute-aware duration. tagged clips are pod-ready; untagged
// clips flag caution with a "Tag" action; aiTagged clips wear a `suggest` marker
// (§2.1) and offer a one-click confirm — the human still gates the AI's guess (§8).
const KIND_LABEL: Record<ClipDTO["kind"], string> = {
  commercial: "Commercial",
  bumper: "Bumper",
  station_id: "Station ID",
  psa: "PSA",
  trailer: "Trailer",
  interstitial: "Interstitial",
};

// Keyed loosely because ClipDTO's audience includes "" (unset); the guard below skips it.
const AUDIENCE_LABEL: Record<string, string> = {
  kids: "Kids",
  family: "Family",
  general: "General",
  late_night: "Late night",
};

// The cycle orders. Each ends at "" / 0 — UNSET is a reachable step, not a trap: a chip you
// can only advance through would make a wrongly-tagged clip impossible to blank without
// opening the dialog, and §10's likely error is exactly a mis-tagged clip.
const AUDIENCES = ["kids", "family", "general", "late_night", ""] as const;
// The CYCLE steps by decade (cycling by 1 would be 10 clicks per useful step), bounded to the span
// of TV advertising the catalog holds, then back to unset.
const ERAS = [1950, 1960, 1970, 1980, 1990, 2000, 2010, 2020, 0] as const;

// eraLabel renders an era for display. ⚠ The tagger grounds a LITERAL YEAR from the clip's text
// (§8), so `era` is often a specific year (1996), NOT a decade — and "1996s" is nonsense. Append the
// decade "s" ONLY when the value is a decade boundary (divisible by 10); a specific year renders
// as-is. 1990 → "1990s"; 1996 → "1996". 0/undefined → "" (the caller shows an "era" placeholder).
const eraLabel = (era: number | undefined): string => {
  if (!era) return "";
  return era % 10 === 0 ? `${era}s` : String(era);
};
// ⚠ The hardcoded CATEGORIES cycle list was DELETED (§10 V45a). `category` is now a DERIVED shadow
// of the taxonomy tags, not a directly-editable free string — there is nothing to cycle. Tags are
// shown as read-only badges and edited in the tag dialog, which serves the real vocabulary. (The list
// was also a rule violation: operator-editable data hardcoded on the FE — see the no-hardcode rule.)

const next = <T,>(list: readonly T[], current: T): T => list[(list.indexOf(current) + 1) % list.length] as T;

// How much this clip has actually AIRED — the mock's `usedLine`.
//
// ⚠ The verb is "aired", not "played", deliberately (fixed after a live review: "played" read as
// "have I watched this", and the Play button on the card visibly did not move the number). The count
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

// A tag chip you can click to advance. Styled to match Badge (mono/uppercase, §2.2) rather
// than composing it, because a <button> inside a <span>-shaped Badge would nest interactive
// content in a label. The hover border is the affordance: a chip that looks identical to a
// static badge would never invite the click.
const CycleChip = ({
  label,
  title,
  unset,
  onClick,
}: {
  label: string;
  title: string;
  unset?: boolean;
  onClick: () => void;
}) => (
  <button
    type="button"
    onClick={onClick}
    title={title}
    // The title doubles as the accessible name: "1990s" alone does not say it is editable.
    aria-label={title}
    className={cn(
      // ⚠ cursor-pointer: Tailwind v4's Preflight no longer sets it on <button> (v3 did), so this
      // chip showed an ARROW — while the comment above claimed "the hover border is the
      // affordance", which was written when the cursor changed too. A chip that looks like a
      // static badge and does not even change the pointer invites nothing.
      "inline-flex w-fit cursor-pointer items-center rounded-sm border border-border px-1.5 py-0.5 font-medium font-mono text-2xs uppercase tracking-wide transition-colors hover:border-static-400 hover:text-static-200",
      // ⚠ An UNSET chip is a quiet invitation, not an alarm. Three chips shouting
      // "AUDIENCE?" beside a "Untagged" badge says the same thing four times and makes an
      // untagged clip look broken rather than merely unfinished — the badge is the signal,
      // these are the controls.
      //
      // ⚠ static-400, NOT static-500. The token file marks 500 "DISABLED-only + decorative
      // glyphs (2.94:1 — fails for info text)", and the a11y gate caught exactly that: this
      // is interactive TEXT, so it needs AA. The dashed border carries the "unset" signal
      // instead, which colour alone should not have been doing anyway.
      unset && "border-dashed text-static-400",
    )}
  >
    {label}
  </button>
);

// ClipFrame — the thumbnail, its overlays, and the hover preview (V39).
//
// Its own component because it is the only stateful part of an otherwise pure card, and inlining
// a `useState` would make every card in a 200-card grid re-render on any one card's hover.
//
// ⚠ **The animation is a generated ASSET, not the clip streamed on hover.** Playing the source on
// hover was the cheaper build and the wrong one at catalog scale: a grid would open a range
// request per hovered card against files tens of megabytes each, to show six seconds. The webp is
// ~150KB and already the right length. See internal/filler/preview.go.
const ClipFrame = ({
  clip,
  selected,
  onToggleSelect,
  onPlay,
}: {
  // ⚠ `thumbImage` is REQUIRED here even though it is optional on ClipDTO. The frame exists only
  // when there is artwork to put in it, and stating that in the type is what lets this component
  // hand the record straight to <Image> — the caller's `clip.thumbImage &&` guard is then the one
  // place the absence is handled, rather than a narrowing TypeScript cannot carry across the
  // component boundary and a redundant re-check in here.
  clip: ClipDTO & { thumbImage: NonNullable<ClipDTO["thumbImage"]> };
  selected?: boolean;
  onToggleSelect?: () => void;
  onPlay?: () => void;
}) => {
  // ⚠ **Unmounted on leave, and that is what makes the animation RESTART.** An animated WebP
  // begins at frame 0 when it decodes and then runs on its own — the browser keeps it going while
  // the element lives, whether or not anyone is looking. Keeping it mounted and merely hiding it
  // meant the second hover picked the loop up wherever it happened to be, usually mid-advert.
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
  // ⚠ Keys on `hoverImage`, not the retired `preview` path (V52 phase 8). Same question — "is
  // there an animation to show?" — asked of the record that now answers it.
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
      className="group relative -mx-3 -mt-3 aspect-video overflow-hidden rounded-t-[inherit] bg-static-800"
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      // Focus counts as hover so a keyboard user tabbing to the play button sees the same preview
      // a mouse user does. `onBlur` mirrors it for the same restart reason.
      onFocus={() => setHovered(true)}
      onBlur={() => setHovered(false)}
    >
      {/* ⚠ ONE path since V52 phase 8. The legacy `/v1/filler/thumb/{hash}` fallback that stood retired-ok
          here through the migration window is retired with its route — a clip's artwork is an
          image-service image (§22).

          Empty alt, deliberately: the clip's name is the very next element, so a description here
          would have a screen reader announce the same clip twice. The frame is decoration for a
          label that is already present.

          No `priority`: a catalog is hundreds of cards, and <Image> defaults to lazy + async,
          which is exactly right below the fold. */}
      <Image
        image={clip.thumbImage}
        alt=""
        sizes="(max-width: 640px) 45vw, 220px"
        className="size-full object-cover"
      />
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
        <Image
          image={clip.hoverImage}
          alt=""
          sizes="(max-width: 640px) 45vw, 220px"
          className="absolute inset-0 size-full object-cover"
          fallback={null}
        />
      )}

      {/* Overlays (V35b, the mock's card): duration bottom-right, quality top-right, select
          top-left. ⚠ Each sits on a scrim (`bg-static-900/80`) rather than directly on the frame
          — a thumbnail is arbitrary video, so text with no backing plate has NO guaranteed
          contrast against it. That is a legibility rule, not a style choice. */}
      <span className="absolute right-1.5 bottom-1.5 rounded bg-static-900/80 px-1.5 py-0.5 font-mono text-2xs text-static-100 tabular-nums">
        {formatClipDuration(clip.durationMs)}
      </span>
      {clip.quality && (
        // ⚠ `role="img"` is load-bearing, not decoration. `aria-label` on a BARE span is ignored —
        // ARIA only permits naming on an element that has a role, and the chip-row version below
        // gets one from Badge. Without it the label silently does nothing and "1080P" is announced
        // as letter-spaced shouting, which is the whole reason the label exists.
        <span
          role="img"
          className="absolute top-1.5 right-1.5 rounded bg-static-900/80 px-1.5 py-0.5 font-mono text-2xs text-static-100"
          title="Resolution, from the clip's video height"
          aria-label={`Resolution ${clip.quality}`}
        >
          {clip.quality}
        </span>
      )}
      {onToggleSelect && (
        <Checkbox
          checked={Boolean(selected)}
          onChange={onToggleSelect}
          className="absolute top-1.5 left-1.5 size-4 accent-signal"
          aria-label={`Select ${clip.name}`}
        />
      )}

      {/* The play button — the mock's 58px amber disc (line 982 of the desktop prototype),
          scaled to this smaller frame. Centred over the preview.

          ⚠ A real <button>, not a click handler on the frame. The whole card is not clickable
          (its chips and actions are), so a bare div-with-onClick here would be an invisible
          target with no keyboard route and no accessible name.

          ⚠ Always in the DOM, revealed on hover/focus by opacity — NOT conditionally rendered.
          A button that only exists on hover cannot be reached by Tab, so a keyboard user could
          never play a clip. `group-focus-within` is what makes tabbing to it reveal it. */}
      {onPlay && (
        <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
          {/* The app's `Button` primitive, not a hand-rolled <button>: the pointer cursor (which
              Tailwind v4's Preflight stopped giving <button>), the focus ring and the disabled
              handling all live there once. Only the disc SHAPE is local. */}
          <Button
            variant="ghost"
            size="icon"
            onClick={onPlay}
            // The name says WHICH clip: a grid of buttons all called "Play" is unusable by voice
            // control and meaningless in a screen reader's element list.
            aria-label={`Play ${clip.name}`}
            title={`Play ${clip.name}`}
            className={cn(
              "pointer-events-auto size-11 rounded-full bg-signal/90 text-static-950 opacity-0 shadow-lg transition-opacity hover:bg-signal",
              "focus-visible:opacity-100 group-focus-within:opacity-100 group-hover:opacity-100",
              // Visible focus ring against arbitrary video underneath.
              "focus-visible:ring-static-100 focus-visible:ring-offset-0",
            )}
          >
            {/* ⚠ Nudged right by a pixel: a triangle's optical centre is left of its bounding
                box, so a mathematically centred play glyph reads as sitting too far left. The
                mock does the same thing with `padding-left:4px`. */}
            <Play className="ml-0.5 size-5 fill-current" aria-hidden />
          </Button>
        </div>
      )}
    </div>
  );
};

const ClipCard = ({
  clip,
  onConfirmTags,
  onConfirmEra,
  onTag,
  onPin,
  onCycle,
  onSplit,
  splitPending,
  selected,
  onToggleSelect,
  onPlay,
  className,
}: ClipCardProps) => (
  <Card
    className={cn(
      "flex flex-col gap-2.5 p-3",
      // A selected card is outlined rather than tinted: the grid is thumbnails, and a wash over
      // them changes what the frame looks like, which is the one thing an operator is scanning.
      selected && "ring-1 ring-signal",
      className,
    )}
  >
    {/* The extracted frame (V17b), served by V30. Rendered ONLY when one exists.
        ⚠ A placeholder box for every clip without a thumbnail would be the wrong default: on a
        Tunarr-backed install, or one where ffmpeg never ran, that is the ENTIRE catalog, and a
        grid of identical grey rectangles reads as a broken page rather than an absent nicety.
        Absence is the honest rendering — the card without a frame is exactly what shipped
        before this phase, which is a design that already works. */}
    {clip.thumbImage && (
      <ClipFrame
        clip={{ ...clip, thumbImage: clip.thumbImage }}
        selected={selected}
        onToggleSelect={onToggleSelect}
        onPlay={onPlay}
      />
    )}

    <div className="flex items-start justify-between gap-2">
      {/* ⚠ The checkbox and duration move ONTO the thumbnail above — but only when there is
          one. A clip with no extracted frame renders no image, so without this fallback the
          overlays would have nowhere to sit and the clip would become unselectable. On a
          Tunarr-backed install that is the entire catalog (see the thumbnail note above). */}
      {!clip.thumbImage && onToggleSelect && (
        <Checkbox
          checked={Boolean(selected)}
          onChange={onToggleSelect}
          className="mt-0.5 size-4 shrink-0 accent-signal"
          aria-label={`Select ${clip.name}`}
        />
      )}
      <p className="min-w-0 flex-1 truncate font-medium text-sm">{clip.name}</p>
      {!clip.thumbImage && (
        <span className="shrink-0 font-mono text-static-400 text-xs tabular-nums">
          {formatClipDuration(clip.durationMs)}
        </span>
      )}
    </div>

    <div className="flex flex-wrap gap-1.5">
      <Badge variant="neutral">{KIND_LABEL[clip.kind]}</Badge>
      {/* ⚠ Era, audience and category render as BUTTONS when the caller can retag and as
          plain badges otherwise — a member sees the same tags without a control that would
          403. Clicking advances to the next value (the mock's cycleEra/cycleAud/cycleCat);
          the dialog stays for anything the cycle cannot reach, e.g. a typed category. */}
      {onCycle ? (
        <CycleChip
          label={clip.era ? eraLabel(clip.era) : "era"}
          unset={!clip.era}
          title={`Click to change the era (now ${clip.era ? eraLabel(clip.era) : "unset"})`}
          onClick={() => onCycle({ era: next(ERAS, (clip.era ?? 0) as (typeof ERAS)[number]) })}
        />
      ) : clip.era ? (
        <Badge variant="neutral">{eraLabel(clip.era)}</Badge>
      ) : null}
      {/* An UNCONFIRMED era (§10 V34): the year is in none of the clip's text signals, so
          the grounding validator refused to persist it. It renders as a question — suggest
          magenta, with a "?" — never as a tag, and pod matching never reads it. */}
      {clip.suggestedEra ? (
        <Badge
          variant="suggest"
          title="AI guess. The year isn't in the source text, so confirm it only if you know it's right."
          aria-label={`Suggested era ${clip.suggestedEra}, unconfirmed AI guess`}
        >
          {`${clip.suggestedEra}s?`}
        </Badge>
      ) : null}
      {onCycle ? (
        <CycleChip
          label={(clip.audience && AUDIENCE_LABEL[clip.audience]) || "audience"}
          unset={!clip.audience}
          title={`Click to change the audience (now ${clip.audience ? AUDIENCE_LABEL[clip.audience] : "unset"})`}
          onClick={() =>
            onCycle({ audience: next(AUDIENCES, (clip.audience ?? "") as (typeof AUDIENCES)[number]) })
          }
        />
      ) : clip.audience ? (
        <Badge variant="neutral">{AUDIENCE_LABEL[clip.audience]}</Badge>
      ) : null}
      {/* Tags (§10 V45a): read-only badges from the taxonomy tag set. No inline "cycle" — a tag must
          be a real taxon, so editing routes to the tag dialog (which serves the vocabulary). The
          headline badge is the primary product leaf (`category`, derived); a "+N" chip signals the
          clip carries more tags without cluttering the card with the full rollup set (beer implies
          alcohol, drinks — matched on, not shown). The dialog shows them all. */}
      {clip.category ? <Badge variant="neutral">{clip.category}</Badge> : null}
      {(() => {
        // Extra LEAF-ish tags beyond the headline: the full set minus the rollups of `category` is not
        // known on the wire, so approximate "more than the headline" by count. A clip with only its
        // category's own lineage shows no "+N"; one tagged on another axis (christmas, psa) shows it.
        const extra = (clip.tags ?? []).filter((t) => t !== clip.category).length;
        return extra > 0 ? (
          <Badge variant="neutral" title="This clip has more tags — open it to see them all">
            +{extra}
          </Badge>
        ) : null;
      })()}
      {!clip.tagged && <Badge variant="caution">Untagged</Badge>}
      {/* Resolution, from the probed video height. Display-only unless an operator sets the
          filler.min_quality floor (off by default), so it is a neutral fact here — NOT a
          warning. Colouring a 480p clip as a problem would invent a policy the install has
          not opted into.
          ⚠ Only when there is NO thumbnail — with one, quality renders as an overlay on the
          frame (V35b, the mock). Dropping this branch entirely would hide the resolution on
          exactly the installs that extract no frames, which is the whole catalog on some. */}
      {!clip.thumbImage && clip.quality ? (
        // aria-label per frontend-design §219: the badge renders mono/uppercase by house
        // style, so a screen reader would otherwise announce letter-spaced shouting.
        <Badge
          variant="neutral"
          title="Resolution, from the clip's video height"
          aria-label={`Resolution ${clip.quality}`}
        >
          {clip.quality}
        </Badge>
      ) : null}
    </div>

    {/* How much this clip has actually aired — the mock's `usedLine`. The API has sent
        playCount/playsCounted/lastPlayedAt since the catalog shipped and nothing rendered
        them, which is why a clip that never plays looked identical to one on every break.
        The wording (and the playsCounted trap) lives in `playsLine`, shared with the list row. */}
    <p className="text-static-400 text-xs">{playsLine(clip)}</p>

    {(onConfirmTags || onConfirmEra || onTag || onPin || onSplit || (onPlay && !clip.thumbImage)) && (
      <div className="flex flex-wrap gap-2">
        {/* ⚠ **Only when there is NO frame to overlay.** With a thumbnail the play control is the
            disc centred on it; duplicating it here would put two "Play X" buttons on one card,
            which is noise on screen and a genuine problem in a screen reader's element list.

            Without a thumbnail there is nowhere for the disc to sit — and on a Tunarr-backed
            install, or one where ffmpeg never ran, that is the ENTIRE catalog. Dropping this
            branch would make the whole feature invisible on exactly those installs. */}
        {onPlay && !clip.thumbImage && (
          <Button variant="outline" size="sm" onClick={onPlay} title={`Play ${clip.name}`}>
            <Play aria-hidden />
            Play
          </Button>
        )}
        {clip.aiTagged && onConfirmTags && (
          <Button variant="outline" size="sm" onClick={onConfirmTags}>
            Confirm tags
          </Button>
        )}
        {/* The era suggestion's confirm door. One click, PATCHes the year as fact — the
            human, not the model, grounds the tag (§10). */}
        {clip.suggestedEra != null && clip.suggestedEra > 0 && onConfirmEra && (
          <Button
            variant="outline"
            size="sm"
            onClick={onConfirmEra}
            title={`Save ${clip.suggestedEra} as this clip's era`}
          >
            {`Confirm ${clip.suggestedEra}`}
          </Button>
        )}
        {/* Offered for EVERY clip, not just untagged ones. A fully-tagged clip can still be
            wrong — §10's likely error is a trailer scanned as a commercial, which arrives
            with era/audience/category filled in and therefore counts as "tagged". Gating the
            edit on `!tagged` left exactly that clip uncorrectable, and kind drives pod role,
            so a wrong one yields structurally wrong pods. */}
        {!clip.aiTagged && onTag && (
          <Button variant="outline" size="sm" onClick={onTag}>
            <Tag aria-hidden />
            {clip.tagged ? "Edit tags" : "Tag clip"}
          </Button>
        )}
        {/* Pin into a channel's filler (P3 cohesion) — the catalog → channel bridge. */}
        {onPin && (
          <Button variant="ghost" size="sm" onClick={onPin}>
            <Pin aria-hidden />
            Use in a channel
          </Button>
        )}
        {/* Compilation splitting (§10 V34): detection runs as a job, then the operator
            REVIEWS the proposed cuts before anything enters the catalog. */}
        {onSplit && (
          <Button
            variant="ghost"
            size="sm"
            disabled={splitPending}
            onClick={onSplit}
            title="Detect commercials inside this compilation and review the cuts"
          >
            <Scissors aria-hidden />
            {splitPending ? "Splitting…" : "Split into clips"}
          </Button>
        )}
      </div>
    )}
  </Card>
);

export { AUDIENCE_LABEL, ClipCard, KIND_LABEL, playsLine };
