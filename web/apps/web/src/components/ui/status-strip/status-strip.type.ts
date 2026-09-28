import type { ReactNode } from "react";
import type { ButtonProps } from "../button";
import type { StatusTone } from "../status-dot";

// The strip's headline state. `idle` is the grey, ringless dot: nothing on air yet, or still
// checking (with `loading`).
type StatusStripTone = "ok" | "error" | "idle";

interface StatusStripItem {
  // Stable React key.
  id: string;
  tone: StatusTone;
  title: ReactNode;
  // One `StatusStripAction`.
  action?: ReactNode;
}

interface StatusStripProps {
  tone: StatusStripTone;
  // Pulses the dot while the headline is still being worked out. Off under reduced motion, which
  // is why the title must say it too ("Checking your channels…").
  loading?: boolean;
  title: ReactNode;
  items?: StatusStripItem[];
  // A trailing call to action at the right edge ("Add your first channel").
  action?: ReactNode;
  className?: string;
}

interface StatusStripActionProps extends Omit<ButtonProps, "size" | "variant"> {
  // The amber fill, for the one item that most needs doing. Outlined otherwise.
  primary?: boolean;
}

export type { StatusStripActionProps, StatusStripItem, StatusStripProps, StatusStripTone };
