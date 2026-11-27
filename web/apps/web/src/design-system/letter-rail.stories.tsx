import { LetterRail } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// LetterRail — "#" then A to Z down the side of a long list. A letter with nothing under it is
// disabled; the letter the reader is at is raised.
const meta = {
  title: "Loomarr Components/LetterRail",
  component: LetterRail,
  args: {
    available: ["#", "A", "B", "C", "D", "H", "M", "N", "P", "S", "T", "W"],
    current: "A",
    onSelect: () => {},
  },
  decorators: [
    (Story) => (
      <div style={{ padding: 24, width: 80 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof LetterRail>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Default, Light };
