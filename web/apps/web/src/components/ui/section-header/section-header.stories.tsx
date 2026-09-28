import type { Meta, StoryObj } from "@storybook/react-vite";
import { SectionHeader, SectionHeaderAction } from "./section-header";

// SectionHeader — a Home section's title, meta and trailing link (#1659 web mock).
const meta = {
  title: "Primitives/SectionHeader",
  component: SectionHeader,
  args: { title: "Tonight" },
  decorators: [
    (Story) => (
      <div className="max-w-[1120px] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SectionHeader>;

type Story = StoryObj<typeof meta>;

const WithLink: Story = {
  args: { children: <SectionHeaderAction>Full guide</SectionHeaderAction> },
};

const WithMeta: Story = {
  args: { title: "Watching now", meta: "3 people" },
};

// The ideas header: meta and a cluster of quiet controls on the right.
const MetaAndControls: Story = {
  args: {
    title: "Channel ideas",
    meta: "Picked from your library and what you've asked for",
    children: <SectionHeaderAction>Different ideas</SectionHeaderAction>,
  },
};

// At phone width the trailing link wraps under the title instead of squeezing it.
const Narrow: Story = {
  args: {
    title: "On the way",
    meta: "3 downloading",
    children: <SectionHeaderAction>All requests</SectionHeaderAction>,
  },
  decorators: [
    (Story) => (
      <div className="w-[240px]">
        <Story />
      </div>
    ),
  ],
};

export default meta;
export { MetaAndControls, Narrow, WithLink, WithMeta };
