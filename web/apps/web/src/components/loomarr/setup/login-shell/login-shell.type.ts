import type { ReactNode } from "react";

interface LoginShellProps {
  /** The sign-in form (or any auth content) rendered inside the centered card. */
  children: ReactNode;
  /** Top and bottom padding in px. The sides are always 24. A tall form passes more. */
  paddingVertical?: number;
}

export type { LoginShellProps };
