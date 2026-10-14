import type { ComponentPropsWithoutRef, ReactNode } from "react";

interface PageHeaderProps extends Omit<ComponentPropsWithoutRef<"header">, "title"> {
  /** The page's only level-one heading. */
  title: ReactNode;
  /** Short context that explains what the page owns. */
  description?: ReactNode;
  /** A live status line under the description, such as Filler's watch line. It brings its own
   * element and top margin, since a block can't sit inside the description's paragraph. */
  status?: ReactNode;
  /** Page-level controls or status. They stack below the copy on narrow screens. */
  actions?: ReactNode;
}

export type { PageHeaderProps };
