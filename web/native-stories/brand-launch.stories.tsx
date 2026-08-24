import { BrandLaunch } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";

const meta = {
  title: "Loomarr/Brand Launch",
  component: BrandLaunch,
  args: { density: "touch", reducedMotion: false },
} satisfies Meta<typeof BrandLaunch>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const ReducedMotion: Story = { args: { reducedMotion: true } };

export default meta;
export { ReducedMotion, Touch, Tv };
