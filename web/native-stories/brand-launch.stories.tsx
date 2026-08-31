import { BrandLaunch } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";

const meta = {
  title: "Loomarr Foundations/Brand Launch",
  component: BrandLaunch,
  args: { density: "touch", reducedMotion: false },
} satisfies Meta<typeof BrandLaunch>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const ReducedMotion: Story = { args: { reducedMotion: true } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, ReducedMotion, Touch, Tv };
