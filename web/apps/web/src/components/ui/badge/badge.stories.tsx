import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge } from "./badge";

// Badge — the tinted chip, and THE component the §2.1 badge rule is about.
//
// Every variant is accent text on that accent's 15% tint. `onair` and `suggest` use their -300
// stops because their base stops fail AA on the composited tint (4.02:1 and 3.86:1) — the rule
// the doc calls "learned the hard way, twice". Design/Palette renders the same maths live.
const meta = {
  title: "Primitives/Badge",
  component: Badge,
  args: { children: "Live" },
} satisfies Meta<typeof Badge>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

// Every variant at once. This is the story that matters: it puts all seven tint/text pairings
// in one frame, so a palette edit that breaks one is a visible diff rather than a number in a
// test log.
const AllVariants: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      {(["neutral", "tune", "lock", "caution", "onair", "suggest", "signal"] as const).map((v) => (
        <Badge key={v} variant={v}>
          {v}
        </Badge>
      ))}
    </div>
  ),
};

// The two that need their -300 stop, isolated — the pairing most likely to regress.
const OnTintStops: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      <Badge variant="onair">on air</Badge>
      <Badge variant="suggest">generating</Badge>
    </div>
  ),
};

export default meta;
export { AllVariants, Default, OnTintStops };
