import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "./button";

// Button — every variant and size (§4.1 Layer 1, §5.1).
//
// ⚠ THREE OF THESE VARIANTS CARRY A CONTRAST CALIBRATION that was learned the hard way and is
// invisible in the markup: `default`, `suggest` and `destructive` all use DARK text on a solid
// accent, because white fails AA on each (suggest 4.12:1, onair 3.91:1 — the latter caught by
// the a11y gate on the danger zone, i.e. in production rather than in review). These stories
// exist so a token nudge that pushes them back under the bar fails a pixel diff and an axe run,
// instead of shipping.
const meta = {
  title: "Primitives/Button",
  component: Button,
  args: { children: "Add a channel" },
} satisfies Meta<typeof Button>;

type Story = StoryObj<typeof meta>;

const Row = ({ children }: { children: React.ReactNode }) => (
  <div className="flex flex-wrap items-center gap-3">{children}</div>
);

// The amber primary — brand and the everyday affirmative action.
const Default: Story = {};

// The AI action (§2.1: suggest is "THE AI color"), reserved for generation so magenta always
// means "this asks the model".
const Suggest: Story = { args: { variant: "suggest", children: "Suggest a lineup" } };

const Destructive: Story = { args: { variant: "destructive", children: "Delete channel" } };
const Outline: Story = { args: { variant: "outline" } };
const Secondary: Story = { args: { variant: "secondary" } };
const Ghost: Story = { args: { variant: "ghost" } };
const Link: Story = { args: { variant: "link", children: "Read the docs" } };

// All variants together — the view that makes an odd one out obvious.
const AllVariants: Story = {
  render: () => (
    <Row>
      {(["default", "suggest", "destructive", "outline", "secondary", "ghost", "link"] as const).map((v) => (
        <Button key={v} variant={v}>
          {v}
        </Button>
      ))}
    </Row>
  ),
};

const AllSizes: Story = {
  render: () => (
    <Row>
      {(["sm", "default", "lg"] as const).map((s) => (
        <Button key={s} size={s}>
          {s}
        </Button>
      ))}
      <Button size="icon" aria-label="Refresh">
        ↻
      </Button>
    </Row>
  ),
};

// Disabled is a real state with its own contrast floor — static-500 is restricted to exactly
// this (§2.1), so it needs a baseline like any other.
const Disabled: Story = {
  render: () => (
    <Row>
      {(["default", "suggest", "destructive", "outline"] as const).map((v) => (
        <Button key={v} variant={v} disabled>
          {v}
        </Button>
      ))}
    </Row>
  ),
};

export default meta;
export { AllSizes, AllVariants, Default, Destructive, Disabled, Ghost, Link, Outline, Secondary, Suggest };
