import { GuideExperience } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";

const meta = {
  title: "Loomarr/Guide Experience",
  component: GuideExperience,
  args: { density: "touch", onRetry: () => undefined, state: "loading" },
} satisfies Meta<typeof GuideExperience>;

type Story = StoryObj<typeof meta>;
const Loading: Story = {};
const Empty: Story = { args: { state: "empty" } };
const ErrorState: Story = { args: { state: "error" } };
const TvOffline: Story = { args: { density: "tv", state: "offline" } };

export default meta;
export { Empty, ErrorState, Loading, TvOffline };
