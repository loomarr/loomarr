import { cn } from "@/lib/utils";
import type { PageHeaderProps } from "./page-header.type";

// PageHeader is the page-edge seam, not just shared heading typography. It owns the gutter,
// rule and responsive action layout so a route cannot accidentally invent a fourth header.
const PageHeader = ({ title, description, actions, className, ...rest }: PageHeaderProps) => (
  <header
    className={cn(
      "flex shrink-0 flex-col gap-4 border-border border-b px-6 py-4 sm:flex-row sm:items-start sm:justify-between",
      className,
    )}
    data-page-header=""
    {...rest}
  >
    <div className="min-w-0 flex-1">
      <h1 className="font-semibold text-xl">{title}</h1>
      {description ? <p className="mt-1 max-w-3xl text-muted-foreground text-sm">{description}</p> : null}
    </div>
    {actions ? <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div> : null}
  </header>
);

export { PageHeader };
