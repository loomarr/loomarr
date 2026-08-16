import type { ComponentProps } from "react";
import type { Button } from "@/components/ui/button";

interface RunButtonProps extends Omit<ComponentProps<typeof Button>, "onClick" | "children"> {
  // True while the action is in flight. Compute it with `useRunFeedback` unless the surface
  // genuinely has a reliable server-side signal — see that hook for why it usually does not.
  busy: boolean;
  onRun: () => void;
  label?: string; // default "Run now"
  busyLabel?: string; // default "Running…"
}

export type { RunButtonProps };
