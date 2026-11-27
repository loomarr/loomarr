import type { Meta, StoryObj } from "@storybook/react-vite";
import { IdleSnow } from "./idle-snow";

// The idle snow fills its parent, so the story frames it. Under the gallery's reduced motion it draws
// nothing and the baseline is the empty frame; the motion suite shows it moving.
const meta = {
  title: "Shell/IdleSnow",
  component: IdleSnow,
  decorators: [
    (Story) => (
      <div style={{ background: "var(--color-static-950)", height: 160, position: "relative", width: 320 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof IdleSnow>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

export default meta;
export { Default };
