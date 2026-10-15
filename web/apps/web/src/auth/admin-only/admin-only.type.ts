import type { ReactNode } from "react";

interface AdminOnlyProps {
  /** The admin page. Never mounted for a member, so none of its reads are made. */
  children: ReactNode;
}

export type { AdminOnlyProps };
