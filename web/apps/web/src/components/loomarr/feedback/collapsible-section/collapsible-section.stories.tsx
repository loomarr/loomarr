import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge } from "@/components/ui";
import { widthFrame } from "@/test/story-utils";
import { CollapsibleSection } from "./collapsible-section";

// CollapsibleSection — the workbench accordion. Its story set is the visual contract: a
// collapsed header and an expanded body, plus a header with a trailing slot.
const meta = {
  title: "Feedback/CollapsibleSection",
  component: CollapsibleSection,
  decorators: [widthFrame(560)],
} satisfies Meta<typeof CollapsibleSection>;

type Story = StoryObj<typeof meta>;

const body = (
  <div className="flex flex-col gap-3 text-muted-foreground text-sm">
    <p>The body slides open from the header — height-agnostic, reduced-motion aware.</p>
    <p>Everything inside stays fully interactive once expanded.</p>
  </div>
);

const Collapsed: Story = {
  args: {
    title: "Programming rules",
    description: "How this channel picks and orders what plays.",
    children: body,
  },
};

const Expanded: Story = {
  args: {
    title: "Programming rules",
    description: "How this channel picks and orders what plays.",
    defaultOpen: true,
    children: body,
  },
};

const WithTrailing: Story = {
  args: {
    title: "Lineup",
    description: "Add, remove, and reorder the titles this channel plays.",
    defaultOpen: true,
    trailing: <Badge variant="neutral">4 titles</Badge>,
    children: body,
  },
};

export default meta;
export { Collapsed, Expanded, WithTrailing };
