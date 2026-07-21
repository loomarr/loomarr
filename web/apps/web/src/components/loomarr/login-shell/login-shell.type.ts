import type { ReactNode } from "react";

interface LoginShellProps {
  /** The sign-in form (or any auth content) rendered inside the centered card. */
  children: ReactNode;
  className?: string;
}

export type { LoginShellProps };
