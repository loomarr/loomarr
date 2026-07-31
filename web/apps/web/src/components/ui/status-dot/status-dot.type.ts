import type { ComponentPropsWithoutRef } from "react";

type StatusTone = "live" | "pending" | "ok" | "warn" | "error" | "off";

// Extends span props so callers can attach the test/data hooks a state indicator naturally
// wants (`data-testid`, `data-onair`) without the primitive enumerating them.
interface StatusDotProps extends Omit<ComponentPropsWithoutRef<"span">, "aria-label"> {
  tone: StatusTone;
  // What the dot MEANS, for assistive tech. Required rather than optional: a bare coloured
  // circle conveys nothing without it, and the a11y gate treats an unlabelled one as a
  // violation. Passing "" opts out explicitly, for a dot beside text that already says it.
  label: string;
}

export type { StatusDotProps, StatusTone };
