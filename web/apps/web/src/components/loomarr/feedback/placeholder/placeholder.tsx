import { Stack, Text } from "@loomarr/design-system";
import { StatePanel } from "@loomarr/ui";
import type { PlaceholderProps } from "./placeholder.type";

// Placeholder page content: the 404 and the stub routes under _authed. Every list gets an
// EmptyState with exactly one next action (§6); this is the same family for a whole page.
const Placeholder = ({ title, hint }: PlaceholderProps) => (
  <Stack height="100%">
    <Stack
      borderBottomWidth={1}
      borderColor="$borderDecorative"
      paddingHorizontal={24}
      paddingVertical={16}
      render="header"
    >
      <Text as="h1" textRole="title">
        {title}
      </Text>
    </Stack>
    <Stack align="center" flexBasis="0%" flexGrow={1} flexShrink={1} justify="center" padding={24}>
      <StatePanel description={hint} kind="empty" title="Nothing here" />
    </Stack>
  </Stack>
);

export { Placeholder };
