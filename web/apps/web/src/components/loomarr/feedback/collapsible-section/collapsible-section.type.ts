import type { ReactNode } from "react";

interface CollapsibleSectionProps {
  // The section heading (rendered in the clickable header).
  title: string;
  // A one-line muted sub-header under the title (optional).
  description?: string;
  // An icon rendered before the title (e.g. the Sparkles for an AI section).
  icon?: ReactNode;
  // A slot on the right of the header, before the chevron (e.g. a count badge or a
  // "dirty" dot). Clicks inside it should stopPropagation if they shouldn't toggle.
  trailing?: ReactNode;
  // Whether the section starts open (uncontrolled). Ignored when `open` is provided.
  defaultOpen?: boolean;
  // Controlled open state — provide with onOpenChange to drive it from a parent.
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  children: ReactNode;
  className?: string;
}

export type { CollapsibleSectionProps };
