import { Stack, Text } from "@loomarr/design-system";
import type { PageHeaderProps } from "./page-header.type";

// PageHeader is the page-edge seam, not just shared heading typography. It owns the gutter,
// rule and responsive action layout so a route cannot accidentally invent a fourth header.
const PageHeader = ({ title, description, status, actions, ...rest }: PageHeaderProps) => (
  <Stack
    align={{ base: "stretch", sm: "start" }}
    borderBottomWidth={1}
    borderColor="$borderDecorative"
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
    <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minWidth={0}>
      <Text as="h1" textRole="title">
        {title}
      </Text>
      {description ? (
        <Stack marginTop={4} maxWidth={768}>
          <Text as="p" textRole="compact" tone="secondary">
            {description}
          </Text>
        </Stack>
      ) : null}
      {status}
    </Stack>
    {actions ? (
      <Stack align="center" direction="row" flexShrink={0} gap="2" minWidth={0} wrap>
        {actions}
      </Stack>
    ) : null}
  </Stack>
);

export { PageHeader };
