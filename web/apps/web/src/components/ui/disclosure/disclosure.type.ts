import type { ReactNode } from "react";

interface DisclosureProps {
  /**
   * ⚠ Forwarded as-is, INCLUDING undefined — that is what selects uncontrolled mode. Coercing it
   * (`open ?? false`) pins every uncontrolled disclosure shut, which is the failure this prop's
   * sibling in `CollapsibleSection` already records.
   */
  open?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (open: boolean) => void;
  children: ReactNode;
  className?: string;
}

interface DisclosureTriggerProps {
  /**
   * The accessible name, required and written as a full sentence ("Show what happened to
   * Coca-Cola 1985").
   *
   * ⚠ Required because the trigger renders a bare chevron. A control whose only content is an
   * icon has no accessible name at all unless one is supplied, and a row of unnamed chevrons is
   * a list of buttons called "button".
   */
  label: string;
  className?: string;
}

interface DisclosurePanelProps {
  children: ReactNode;
  className?: string;
}

export type { DisclosurePanelProps, DisclosureProps, DisclosureTriggerProps };
