import { Stack, Text } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import type { PageHeaderProps } from "./page-header.type";

// PageHeader is the page-edge seam, not just shared heading typography. It owns the gutter,
// rule and responsive action layout so a route cannot accidentally invent a fourth header.
const PageHeader = ({ title, description, status, actions, className, ...rest }: PageHeaderProps) => (
  <Stack
    align={{ base: "stretch", sm: "start" }}
    className={cn("border-border border-b", className)}
    data-page-header=""
    direction={{ base: "column", sm: "row" }}
    flexShrink={0}
    gap="4"
    justify={{ base: "start", sm: "between" }}
    paddingHorizontal={24}
    paddingVertical={16}
    render="header"
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
  </Stack>
);

export { PageHeader };
