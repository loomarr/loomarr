import type { PlayoutStatus } from "@loomarr/api";

interface PlayoutPanelProps {
  // The whole live-playout picture (GET /v1/playout/status): GPU/LLM header + one row per channel.
  // Optional so the panel can render its loading and "not running" states before data arrives.
  status?: PlayoutStatus;
  loading?: boolean;
  // The dashboard keeps the concise default; Settings names the card as live, read-only state.
  title?: string;
  className?: string;
}

export type { PlayoutPanelProps };
