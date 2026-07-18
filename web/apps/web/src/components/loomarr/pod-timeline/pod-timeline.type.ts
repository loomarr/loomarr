import type { Clip } from "../clip-card";

type PodMatch = "matched" | "fallback-widened" | "bumper-card-only";

interface PodTimelineProps {
  clips: Clip[];
  match?: PodMatch;
  era?: number;
  audience?: string;
  className?: string;
}

export type { PodMatch, PodTimelineProps };
