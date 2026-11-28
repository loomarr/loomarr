import { Scrim, Surface, Text, ThemeScope } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// Scrim: ink over a washed-out frame, fading toward one edge, with and without a soft top and bottom.

const meta = {
  title: "Design System/Scrim",
  component: Scrim,
  args: { feather: 64, from: "right", soften: 32 },
  render: (args) => (
    <div style={{ background: "#f2ead8", padding: "64px 0", width: 480 }}>
      <ThemeScope theme="dark">
        <Surface
          alignSelf="flex-start"
          backgroundColor="transparent"
          borderWidth={0}
          padding={24}
          paddingRight={args.from === "right" ? 88 : 24}
          paddingTop={args.from === "top" ? 88 : 24}
          position="relative"
        >
          <Scrim {...args} />
          <Surface backgroundColor="transparent" borderWidth={0} position="relative">
            <Text textRole="heading">The diner years</Text>
          </Surface>
        </Surface>
      </ThemeScope>
    </div>
  ),
} satisfies Meta<typeof Scrim>;

type Story = StoryObj<typeof meta>;

const Side: Story = {};
const Foot: Story = { args: { from: "top", soften: 0 } };

export { Foot, Side };
export default meta;
