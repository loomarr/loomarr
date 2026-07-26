import type { ReactNode } from "react";

// Step-level state (config-design §6). `skipped` exists so an optional step reads
// neutral rather than red — skipping AI must never shame the operator.
type WizardStepStatus = "done" | "current" | "pending" | "skipped";

interface WizardSubItem {
  id: string;
  label: string;
}

interface WizardStep {
  id: string;
  title: string;
  optional?: boolean;
  // Second-level items shown indented under this step in the rail when it is current
  // (e.g. the individual connections). Clicking one reveals its section in the content.
  subItems?: WizardSubItem[];
}

interface WizardShellProps {
  steps: WizardStep[];
  currentId: string;
  statusById: Record<string, WizardStepStatus>;
  // The sub-item the current step has focused, and the click handler — the rail and the
  // step body share this so clicking a sub-item reveals its section (§13 delight).
  activeSubItem?: string;
  onSubItem?: (id: string) => void;
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

export type { WizardShellProps, WizardStep, WizardStepStatus, WizardSubItem };
