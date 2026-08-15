interface CreateLocalPanelProps {
  // Refetch the users list after a successful create.
  onCreated?: () => void;
  initiallyOpen?: boolean;
  className?: string;
}

export type { CreateLocalPanelProps };
