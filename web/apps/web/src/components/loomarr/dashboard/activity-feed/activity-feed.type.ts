import type { Activity } from "@loomarr/api/models/activity";

interface ActivityFeedProps {
  /** Newest first, from GET /v1/activity. */
  entries: Activity[];
  /** Clock for the relative times, injectable so tests and stories are deterministic. */
  now?: number;
  /** Open the surface that owns an entry. The raw subject id stays out of the row. */
  onOpen?: (entry: Activity) => void;
  className?: string;
}

export type { ActivityFeedProps };
