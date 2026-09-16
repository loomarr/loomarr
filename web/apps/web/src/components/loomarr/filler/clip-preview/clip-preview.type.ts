import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import type { ReactNode } from "react";

type ClipPreviewProps = {
  clip: Pick<ClipDTO, "hash" | "name">;
  onPlaybackStart?: () => void;
  leading?: ReactNode;
};

export type { ClipPreviewProps };
