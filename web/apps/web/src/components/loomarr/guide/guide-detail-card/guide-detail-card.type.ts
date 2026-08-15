import type { GuideAiring } from "@loomarr/api/models/guideAiring";

interface GuideDetailCardProps {
  // The block being inspected. Null renders nothing — the caller can mount this
  // unconditionally rather than guarding at every use.
  airing: GuideAiring | null;
  className?: string;
}

export type { GuideDetailCardProps };
