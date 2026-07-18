import type { ReactNode } from "react";

// Step-level state (config-design §6). `skipped` exists so an optional step reads
// neutral rather than red — skipping AI must never shame the operator.
type WizardStepStatus = "done" | "current" | "pending" | "skipped";

interface WizardStep {
  id: string;
  title: string;
  optional?: boolean;
}

interface WizardShellProps {
  steps: WizardStep[];
  currentId: string;
  statusById: Record<string, WizardStepStatus>;
  // Heading + lede for the step being shown.
  title: string;
  description?: string;
  children: ReactNode;
  onBack?: () => void;
  // Steps whose primary action is generic ("Continue"). A step with its own semantics —
  // Bootstrap's "Create admin" — renders its button inside its own form instead.
  onNext?: () => void;
  onSkip?: () => void;
  nextLabel?: string;
  nextDisabled?: boolean;
  busy?: boolean;
  className?: string;
}

export type { WizardShellProps, WizardStep, WizardStepStatus };
