import { Text } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import type { PageHeaderProps } from "./page-header.type";

// PageHeader is the page-edge seam, not just shared heading typography. It owns the gutter,
// rule and responsive action layout so a route cannot accidentally invent a fourth header.
const PageHeader = ({ title, description, status, actions, className, ...rest }: PageHeaderProps) => (
  <header
    className={cn(
      "border-border flex shrink-0 flex-col gap-4 border-b px-6 py-4 sm:flex-row sm:items-start sm:justify-between",
      className,
    )}
    data-page-header=""
    {...rest}
  >
    <div className="min-w-0 flex-1">
      <Text as="h1" textRole="title">
        {title}
      </Text>
      {description ? (
        <Text as="p" className="mt-1 max-w-3xl" textRole="compact" tone="secondary">
          {description}
        </Text>
      ) : null}
      {status}
    </div>
    {actions ? (
      <div className="flex w-full min-w-0 flex-wrap items-center gap-2 sm:w-auto sm:shrink-0">{actions}</div>
    ) : null}
  </header>
);

export { PageHeader };
