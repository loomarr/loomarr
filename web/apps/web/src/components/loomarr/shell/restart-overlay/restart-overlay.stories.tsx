import type { Meta, StoryObj } from "@storybook/react-vite";
import { RestartOverlay } from "./restart-overlay";

// ⚠ The overlay is `fixed inset-0`, which positions it against the VIEWPORT — so it
// renders outside `#storybook-root`, which is what the visual harness screenshots. Without
// a containing block the baseline captures an empty sliver.
//
// `transform` establishes one (CSS: a transformed ancestor becomes the containing block
// for fixed descendants), so the story frames the overlay exactly as the app shell does at
// full size. Production is untouched — there is no transformed ancestor there.
const meta = {
  title: "Shell/RestartOverlay",
  component: RestartOverlay,
  args: { restarting: true },
  decorators: [
    (Story) => (
      <div style={{ position: "relative", transform: "translateZ(0)", width: 600, height: 320 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof RestartOverlay>;

type Story = StoryObj<typeof meta>;

const Restarting: Story = {};

// ⚠ Success is stated, not implied by disappearance.
const CameBack: Story = {
  args: { restarting: false, justCameBack: true },
};

// The app never came back: this does NOT fade, because the operator has to act.
const NeverCameBack: Story = {
  args: { restarting: false, failed: "Loomarr hasn't come back. Check the container or service." },
};

export default meta;
export { CameBack, NeverCameBack, Restarting };
