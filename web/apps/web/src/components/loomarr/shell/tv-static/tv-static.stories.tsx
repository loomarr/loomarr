import type { Meta, StoryObj } from "@storybook/react-vite";
import { TvStatic } from "./tv-static";

// The CRT snow (§1). It is absolute inset-0, so the story frames it in a relative box on
// the app background. The idle snow renders nothing under reduced motion, so Default is tagged
// `motion`: the gallery shoots it with motion allowed, its seeded tile frozen at frame 0.
const meta = {
  title: "Shell/TvStatic",
  component: TvStatic,
  decorators: [
    (Story) => (
      <div className="bg-background relative h-40 w-80">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof TvStatic>;

type Story = StoryObj<typeof meta>;

const Default: Story = { tags: ["motion"] };

// The tuner's channel-switch snow (#1620): dense and soft at half strength. Unlike the idle snow it
// is present under reduced motion, as a still (seeded) field, so the baseline shows it.
const Wash: Story = { args: { variant: "wash" } };

export default meta;
export { Default, Wash };
