import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ChecklistItem } from "./checklist-item";

// The wizard / Settings check row (§3, §6): pending · running · pass · fail(+hint+doc).
const meta = {
  title: "Loomarr/ChecklistItem",
  component: ChecklistItem,
  decorators: [widthFrame(420)],
} satisfies Meta<typeof ChecklistItem>;

type Story = StoryObj<typeof meta>;

const Pending: Story = { args: { name: "Media server reachable", status: "pending" } };
const Running: Story = { args: { name: "Media server reachable", status: "running" } };
const Pass: Story = { args: { name: "Media server reachable", status: "pass" } };
const Fail: Story = {
  args: {
    name: "Tunarr reachable",
    status: "fail",
    hint: "Connection refused at http://tunarr:8000. Check the URL and that Tunarr is running.",
    docHref: "/help/connections#tunarr",
  },
};

export default meta;
export { Fail, Pass, Pending, Running };
