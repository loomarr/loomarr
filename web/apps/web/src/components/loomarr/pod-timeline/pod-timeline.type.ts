import type { ClipDTO } from "@loomarr/api";

type PodMatch = "matched" | "fallback-widened" | "bumper-card-only";

interface PodTimelineProps {
  clips: ClipDTO[];
  match?: PodMatch;
  era?: number;
  audience?: string;
  className?: string;
}

export type { PodMatch, PodTimelineProps };
