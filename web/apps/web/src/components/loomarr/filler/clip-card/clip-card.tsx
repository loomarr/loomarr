import type { ClipDTO } from "@loomarr/api";
import { clipThumbURL, formatClipDuration, formatRelative } from "@loomarr/core";
import { Pin, Scissors, Sparkles, Tag } from "lucide-react";
import { Badge, Button, Card } from "@/components/ui";
import { cn } from "@/lib";
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
// Decades, not years: `era` is rendered "1990s" and matched by decade, so cycling by 1 would
// be 10 clicks per useful step. Bounded to the span of television advertising the catalog
// actually holds, then back to unset.
const ERAS = [1950, 1960, 1970, 1980, 1990, 2000, 2010, 2020, 0] as const;
// ⚠ NOT an enum on the wire — `category` is a free-string the AI tagger fills from the
// clip's text, so this list is the common set for CYCLING only. The dialog remains the way
// to type anything else, which is why the chip renders a value it cannot cycle to.
const CATEGORIES = ["food", "toys", "auto", "retail", "media", "service", ""] as const;

const next = <T,>(list: readonly T[], current: T): T => list[(list.indexOf(current) + 1) % list.length] as T;

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
      "inline-flex w-fit items-center rounded-sm border border-border px-1.5 py-0.5 font-medium font-mono text-2xs uppercase tracking-wide transition-colors hover:border-static-400 hover:text-static-200",
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
    {clip.thumbnail && (
      <div className="-mx-3 -mt-3 aspect-video overflow-hidden rounded-t-[inherit] bg-static-800">
        <img
          src={clipThumbURL(clip.path)}
          // Empty alt, deliberately: the clip's name is the very next element, so a
          // description here would have a screen reader announce the same clip twice. The
          // frame is decoration for a label that is already present.
          alt=""
          className="size-full object-cover"
          // A catalog is hundreds of cards; without this every frame is fetched on mount.
          loading="lazy"
        />
      </div>
    )}

    <div className="flex items-start justify-between gap-2">
      {/* ⚠ A real checkbox input, not a styled div: bulk selection has to be keyboard-reachable
          and announce its state, and the grid can hold hundreds of these. It renders only when
          the caller passed a handler, so a member sees the card without a control that would
          403 on use. */}
      {onToggleSelect && (
        <input
          type="checkbox"
          checked={Boolean(selected)}
          onChange={onToggleSelect}
          className="mt-0.5 size-4 shrink-0 accent-signal"
          aria-label={`Select ${clip.name}`}
        />
      )}
      <p className="min-w-0 flex-1 truncate font-medium text-sm">{clip.name}</p>
      <span className="shrink-0 font-mono text-static-400 text-xs tabular-nums">
        {formatClipDuration(clip.durationMs)}
      </span>
    </div>

    <div className="flex flex-wrap gap-1.5">
      <Badge variant="neutral">{KIND_LABEL[clip.kind]}</Badge>
      {/* ⚠ Era, audience and category render as BUTTONS when the caller can retag and as
          plain badges otherwise — a member sees the same tags without a control that would
          403. Clicking advances to the next value (the mock's cycleEra/cycleAud/cycleCat);
          the dialog stays for anything the cycle cannot reach, e.g. a typed category. */}
      {onCycle ? (
        <CycleChip
          label={clip.era ? `${clip.era}s` : "era"}
          unset={!clip.era}
          title={`Click to change the era (now ${clip.era ? `${clip.era}s` : "unset"})`}
          onClick={() => onCycle({ era: next(ERAS, (clip.era ?? 0) as (typeof ERAS)[number]) })}
        />
      ) : clip.era ? (
        <Badge variant="neutral">{`${clip.era}s`}</Badge>
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
      {onCycle ? (
        <CycleChip
          label={clip.category || "category"}
          unset={!clip.category}
          title={`Click to change the category (now ${clip.category || "unset"})`}
          onClick={() =>
            onCycle({ category: next(CATEGORIES, (clip.category ?? "") as (typeof CATEGORIES)[number]) })
          }
        />
      ) : clip.category ? (
        <Badge variant="neutral">{clip.category}</Badge>
      ) : null}
      {clip.aiTagged && (
        <Badge variant="suggest">
          <Sparkles className="mr-1 size-3" aria-hidden />
          AI-tagged
        </Badge>
      )}
      {!clip.tagged && !clip.aiTagged && <Badge variant="caution">Untagged</Badge>}
      {/* Resolution, from the probed video height. Display-only unless an operator sets the
          filler.min_quality floor (off by default), so it is a neutral fact here — NOT a
          warning. Colouring a 480p clip as a problem would invent a policy the install has
          not opted into. */}
      {clip.quality ? (
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
        ⚠ playsCounted:false is NOT zero plays. It means this install cannot OBSERVE
        airings (Tunarr-backed playout owns the stream), so "0 plays" would be a lie the
        DTO's own comment warns against. It reads as "plays aren't counted here" instead. */}
    <p className="text-static-400 text-xs">
      {!clip.playsCounted
        ? "Plays aren't counted on this setup"
        : clip.playCount === 0
          ? "Never played"
          : `${clip.playCount} ${clip.playCount === 1 ? "play" : "plays"}${
              clip.lastPlayedAt ? ` · last ${formatRelative(clip.lastPlayedAt)}` : ""
            }`}
    </p>

    {(onConfirmTags || onConfirmEra || onTag || onPin || onSplit) && (
      <div className="flex flex-wrap gap-2">
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

export { ClipCard };
