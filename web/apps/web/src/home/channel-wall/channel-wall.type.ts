import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { GuideLayout } from "@loomarr/core/guide";
import type { NeedsLineItem } from "@loomarr/ui";

interface ChannelWallProps {
  /** The caller's last-tuned channel (GET /v1/household/viewing's continueWatching): first, and marked. */
  lastChannelId?: string | undefined;
  layout: GuideLayout;
  nowMs: number;
  /** An admin's "Needs you" counts above zero (#2785): one quiet line under the header. */
  needs?: NeedsLineItem[];
}

interface AiringArtworkProps {
  airing: Pick<GuideAiring, "backdropImage" | "thumbImage" | "thumbUrl">;
  // The channel the airing is on, for the code-drawn fallback's monogram.
  channel: { name: string; number: number };
  // The `<ServedImage>` `sizes` hint for the slot this fills.
  sizes: string;
}

export type { AiringArtworkProps, ChannelWallProps };
