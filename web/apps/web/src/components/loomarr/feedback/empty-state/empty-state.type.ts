interface EmptyStateProps {
  title: string;
  description?: string;
  action?: { label: string; onClick?: () => void };
  className?: string;
}

export type { EmptyStateProps };
