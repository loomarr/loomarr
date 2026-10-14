import { disjointFillerCriteria } from "@loomarr/fixtures";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { FillerCriteria } from "./filler-criteria";

const withQueryClient: Decorator = (Story) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <Story />
  </QueryClientProvider>
);

const meta = {
  title: "Filler/FillerCriteria",
  component: FillerCriteria,
  args: { onChange: () => {} },
  decorators: [withQueryClient],
} satisfies Meta<typeof FillerCriteria>;

type Story = StoryObj<typeof meta>;

const DisjointDateWindows: Story = {
  args: disjointFillerCriteria,
};

export default meta;
export { DisjointDateWindows };
