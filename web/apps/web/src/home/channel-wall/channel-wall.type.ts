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
  /**
   * Who is watching each channel now, by name (#2785). Given to an admin only: a member's view is never
   * handed names (#3010), so leaving it out is what keeps them off the tiles. Absent also hides the count.
   */
  viewers?: ChannelViewer[];
}

/** One person watching one channel (a person on two devices is one name). */
interface ChannelViewer {
  channelId: string;
  name: string;
  userId: string;
}

interface AiringArtworkProps {
  airing: Pick<GuideAiring, "backdropImage" | "thumbImage" | "thumbUrl">;
  // The channel the airing is on, for the code-drawn fallback's monogram.
  channel: { name: string; number: number };
  // The `<ServedImage>` `sizes` hint for the slot this fills.
  sizes: string;
  // Leave the code-drawn fallback without its monogram (the phone tile, where words sit over all of it).
  wordless?: boolean;
}

export type { AiringArtworkProps, ChannelViewer, ChannelWallProps };
