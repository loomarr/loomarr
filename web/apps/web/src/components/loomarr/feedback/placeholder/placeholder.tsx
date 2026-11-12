import { Stack, Text } from "@loomarr/design-system";
import { StatePanel } from "@loomarr/ui";
import type { PlaceholderProps } from "./placeholder.type";

// Placeholder page content: the 404 and the stub routes under _authed. Every list gets an
// EmptyState with exactly one next action (§6); this is the same family for a whole page.
const Placeholder = ({ title, hint }: PlaceholderProps) => (
  <Stack className="h-full">
    <header className="border-border border-b px-6 py-4">
      <Text as="h1" textRole="title">
        {title}
      </Text>
    </header>
    <Stack align="center" className="p-6" flexBasis="0%" flexGrow={1} justify="center">
      <StatePanel description={hint} kind="empty" title="Nothing here" />
    </Stack>
  </Stack>
);

export { Placeholder };
