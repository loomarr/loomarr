import type { RestartCost } from "@loomarr/api";

interface ServiceControlProps {
  /** What a restart would cost right now, from GET /v1/system/restart. */
  cost: RestartCost;
  /** Restart in place. Only called after the operator confirms. */
  onRestart: () => void;
  /** True once a restart has been asked for, so the button can say what it is doing. */
  restarting?: boolean;
  /** A failure from the last attempt, rendered where it happened. */
  error?: string | null;
  className?: string;
}

export type { ServiceControlProps };
