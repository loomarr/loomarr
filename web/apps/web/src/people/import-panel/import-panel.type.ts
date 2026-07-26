interface ImportPanelProps {
  // Called after a successful import so the caller can refresh the user list.
  onImported?: () => void;
  className?: string;
}

export type { ImportPanelProps };
