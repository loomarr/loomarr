import type { ClipDTO } from "@loomarr/api";
import { clipThumbURL, formatClipDuration } from "@loomarr/core";
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

const ClipCard = ({
  clip,
  onConfirmTags,
  onConfirmEra,
  onTag,
  onPin,
  onSplit,
  splitPending,
  className,
}: ClipCardProps) => (
  <Card className={cn("flex flex-col gap-2.5 p-3", className)}>
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
      <p className="min-w-0 truncate font-medium text-sm">{clip.name}</p>
      <span className="shrink-0 font-mono text-static-400 text-xs tabular-nums">
        {formatClipDuration(clip.durationMs)}
      </span>
    </div>

    <div className="flex flex-wrap gap-1.5">
      <Badge variant="neutral">{KIND_LABEL[clip.kind]}</Badge>
      {clip.era ? <Badge variant="neutral">{`${clip.era}s`}</Badge> : null}
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
      {clip.audience ? <Badge variant="neutral">{AUDIENCE_LABEL[clip.audience]}</Badge> : null}
      {clip.category ? <Badge variant="neutral">{clip.category}</Badge> : null}
      {clip.aiTagged && (
        <Badge variant="suggest">
          <Sparkles className="mr-1 size-3" aria-hidden />
          AI-tagged
        </Badge>
      )}
      {!clip.tagged && !clip.aiTagged && <Badge variant="caution">Untagged</Badge>}
    </div>

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
