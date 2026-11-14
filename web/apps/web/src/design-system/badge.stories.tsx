import { Badge, Stack } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// Badge — the tinted chip, and THE component the §2.1 badge rule is about.
//
// Every variant is accent text on that accent's 15% tint. `onair` and `suggest` use their -300
// stops because their base stops fail AA on the composited tint (4.02:1 and 3.86:1) — the rule
// the doc calls "learned the hard way, twice". Design/Palette renders the same maths live.
const meta = {
  title: "Primitives/Badge",
  component: Badge,
  args: { children: "Live", shape: "square" },
} satisfies Meta<typeof Badge>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

// Every variant at once. This is the story that matters: it puts all seven tint/text pairings
// in one frame, so a palette edit that breaks one is a visible diff rather than a number in a
// test log.
const AllVariants: Story = {
  render: () => (
    <Stack align="center" direction="row" gap="2" wrap>
      {(["neutral", "tune", "lock", "caution", "onair", "suggest", "signal"] as const).map((v) => (
        <Badge key={v} shape="square" tone={v}>
          {v}
        </Badge>
      ))}
    </Stack>
  ),
};

// The two that need their -300 stop, isolated — the pairing most likely to regress.
const OnTintStops: Story = {
  render: () => (
    <Stack align="center" direction="row" gap="2" wrap>
      <Badge shape="square" tone="onair">
        on air
      </Badge>
      <Badge shape="square" tone="suggest">
        generating
      </Badge>
    </Stack>
  ),
};

export default meta;
export { AllVariants, Default, OnTintStops };
