import type { ReactNode } from "react";

interface EmptyStateProps {
  icon?: ReactNode;
  title: string;
  description?: string;
  action?: { label: string; onClick?: () => void };
  className?: string;
}

export type { EmptyStateProps };
