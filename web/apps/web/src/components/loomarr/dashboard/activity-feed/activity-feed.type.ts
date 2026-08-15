import type { Activity } from "@loomarr/api/models/activity";

interface ActivityFeedProps {
  /** Newest first, from GET /v1/activity. */
  entries: Activity[];
  /** Clock for the relative times, injectable so tests and stories are deterministic. */
  now?: number;
  className?: string;
}

export type { ActivityFeedProps };
