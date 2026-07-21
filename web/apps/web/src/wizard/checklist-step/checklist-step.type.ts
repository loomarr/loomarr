interface ChecklistStepProps {
  // The connection block currently revealed (its id === the check name), controlled by the
  // wizard route so the rail's sub-items and the content stay in sync. Undefined = all
  // collapsed.
  openId?: string;
  onToggle?: (id: string) => void;
}

export type { ChecklistStepProps };
