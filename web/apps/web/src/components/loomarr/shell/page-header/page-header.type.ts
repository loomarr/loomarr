import type { ComponentPropsWithoutRef, ReactNode } from "react";

interface PageHeaderProps extends Omit<ComponentPropsWithoutRef<"header">, "title"> {
  /** The page's only level-one heading. */
  title: ReactNode;
  /** Short context that explains what the page owns. */
  description?: ReactNode;
}

export type { PageHeaderProps };
