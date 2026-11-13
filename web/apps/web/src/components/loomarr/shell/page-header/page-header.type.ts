import type { ComponentPropsWithoutRef, ReactNode } from "react";

// The header is a design-system Stack, so it takes the few attributes a page names it by, not every
// DOM attribute a <header> could carry.
interface PageHeaderProps
  extends Pick<ComponentPropsWithoutRef<"header">, "aria-label" | "aria-labelledby" | "className" | "id"> {
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
