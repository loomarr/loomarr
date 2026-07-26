import type { ReactNode } from "react";

interface StatCardProps {
  label: string;
  value: ReactNode;
  // The one-line explanation under the number. Without it a bare count is ambiguous —
  // "12" of what, and does more mean better or worse?
  note: string;
  // Which token colours the number. `neutral` is the resting state; a card only earns a
  // colour when its number means something is waiting or wrong.
  tone?: "neutral" | "onair" | "suggest" | "tune" | "signal";
  className?: string;
}

export type { StatCardProps };
