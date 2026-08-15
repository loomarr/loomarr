import type { PlayoutStatus } from "@loomarr/api/models/playoutStatus";

interface PlayoutPanelProps {
  // The whole live-playout picture (GET /v1/playout/status): GPU/LLM header + one row per channel.
  // Optional so the panel can render its loading and "not running" states before data arrives.
  status?: PlayoutStatus;
  loading?: boolean;
  className?: string;
}

export type { PlayoutPanelProps };
