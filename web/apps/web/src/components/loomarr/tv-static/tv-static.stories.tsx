import type { Meta, StoryObj } from "@storybook/react-vite";
import { TvStatic } from "./tv-static";

// The CRT snow (§1). It is absolute inset-0, so the story frames it in a relative box on
// the app background. Note: under the visual suite's reduced-motion pin it renders EMPTY
// by design — the gallery baseline for this component is intentionally the bare frame.
const meta = {
  title: "Loomarr/TvStatic",
  component: TvStatic,
  decorators: [
    (Story) => (
      <div className="relative h-40 w-80 bg-background">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof TvStatic>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

export default meta;
export { Default };
