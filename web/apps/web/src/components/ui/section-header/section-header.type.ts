import type { useRender } from "@base-ui/react/use-render";
import type { ComponentPropsWithoutRef, ReactNode } from "react";

interface SectionHeaderProps {
  title: string;
  // The quiet count or explanation beside the title ("3 people", "14 titles · 1 new channel").
  meta?: ReactNode;
  // Trailing controls, pushed to the right edge: a `SectionHeaderAction`, or a small cluster of
  // them. The header wraps rather than clipping them at narrow widths.
  children?: ReactNode;
  // Lets the section point `aria-labelledby` at its heading.
  id?: string;
  className?: string;
}

// `render` swaps the element, the same seam as `Button`: pass the router's `<Link to=… />` for
// navigation and keep the header's look. The default is a `<button>`.
interface SectionHeaderActionProps extends ComponentPropsWithoutRef<"button"> {
  render?: useRender.RenderProp;
  // A control rather than a link ("⟳ Different ideas"): the icon leads and the trailing arrow is
  // dropped, since the arrow means "go somewhere".
  icon?: ReactNode;
}

export type { SectionHeaderActionProps, SectionHeaderProps };
