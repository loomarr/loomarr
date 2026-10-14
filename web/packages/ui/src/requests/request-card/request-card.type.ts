import type { RequestEntry } from "@loomarr/core/requests";
import type { ReactNode } from "react";

type RequestCardProps = {
  /** The one thing to do from the list ("Open channel", "Edit and try again"). */
  action?: { label: string; onPress: () => void };
  entry: RequestEntry;
  /** Replaces the status detail, for a failure's reason plus its guidance. */
  hint?: string;
  /** Wall-clock now for the relative date; fixed in stories and tests. */
  nowMs?: number;
  /** Opens the request. Omitted, the card is read-only. */
  onOpen?: () => void;
  trailing?: ReactNode;
};

export type { RequestCardProps };
