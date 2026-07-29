import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { RestartNeededBanner } from "./restart-needed-banner";

const meta = {
  title: "Dashboard/RestartNeededBanner",
  component: RestartNeededBanner,
  args: { pendingKeys: ["DATABASE_URL"], onGoToRestart: () => {} },
  decorators: [widthFrame(760)],
} satisfies Meta<typeof RestartNeededBanner>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

// Two keys edited: the banner lists them rather than counting, since "which one?" is the
// immediate next question.
const SeveralKeys: Story = {
  args: { pendingKeys: ["DATABASE_URL", "LISTEN_ADDR"] },
};

// ⚠ No "renders nothing" story. The visual harness waits for an element to screenshot, so
// a story that deliberately renders nothing times out at 30s. The empty case is asserted
// in restart-needed-banner.test.tsx, where it belongs.

export default meta;
export { Default, SeveralKeys };
