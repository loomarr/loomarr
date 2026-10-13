import type { GuideAiring } from "@loomarr/api/models/guideAiring";

interface AiringArtworkProps {
  airing: Pick<GuideAiring, "thumbImage" | "thumbUrl">;
  // The channel the airing is on, for the code-drawn fallback's monogram.
  channel: { name: string; number: number };
  // The `<Image>` `sizes` hint for the slot this fills.
  sizes: string;
}

export type { AiringArtworkProps };
