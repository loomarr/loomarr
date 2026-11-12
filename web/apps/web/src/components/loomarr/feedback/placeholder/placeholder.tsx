import { Stack } from "@loomarr/design-system";
import { StatePanel } from "@loomarr/ui";
import type { PlaceholderProps } from "./placeholder.type";

// Placeholder page content: the 404 and the stub routes under _authed. Every list gets an
// EmptyState with exactly one next action (§6); this is the same family for a whole page.
const Placeholder = ({ title, hint }: PlaceholderProps) => (
  <Stack className="h-full">
    <header className="border-border border-b px-6 py-4">
      <h1 className="text-xl font-semibold">{title}</h1>
    </header>
    <div className="flex flex-1 items-center justify-center p-6">
      <StatePanel description={hint} kind="empty" title="Nothing here" />
    </div>
  </Stack>
);

export { Placeholder };
