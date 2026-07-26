import type { ClipDTO } from "@loomarr/api";
import { formatClipDuration } from "@loomarr/core";
import { Pin, Sparkles, Tag } from "lucide-react";
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

const ClipCard = ({ clip, onConfirmTags, onTag, onPin, className }: ClipCardProps) => (
  <Card className={cn("flex flex-col gap-2.5 p-3", className)}>
    <div className="flex items-start justify-between gap-2">
      <p className="min-w-0 truncate font-medium text-sm">{clip.name}</p>
      <span className="shrink-0 font-mono text-static-400 text-xs tabular-nums">
        {formatClipDuration(clip.durationMs)}
      </span>
    </div>

    <div className="flex flex-wrap gap-1.5">
      <Badge variant="neutral">{KIND_LABEL[clip.kind]}</Badge>
      {clip.era ? <Badge variant="neutral">{`${clip.era}s`}</Badge> : null}
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

    {(onConfirmTags || onTag || onPin) && (
      <div className="flex flex-wrap gap-2">
        {clip.aiTagged && onConfirmTags && (
          <Button variant="outline" size="sm" onClick={onConfirmTags}>
            Confirm tags
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
      </div>
    )}
  </Card>
);

export { ClipCard };
