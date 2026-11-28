interface EmptyStateProps {
  title: string;
  description?: string;
  action?: { label: string; onClick?: () => void };
}

export type { EmptyStateProps };
