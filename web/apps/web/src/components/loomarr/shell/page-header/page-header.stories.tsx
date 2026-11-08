import { Action } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { PageHeader } from "./page-header";

const meta = {
  title: "Shell/PageHeader",
  component: PageHeader,
} satisfies Meta<typeof PageHeader>;

type Story = StoryObj<typeof meta>;

const TitleOnly: Story = { args: { title: "Dashboard" } };
const WithDescription: Story = {
  args: { title: "Queue", description: "Requests waiting on approval and titles in flight." },
};
const WithActions: Story = {
  args: {
    title: "Channels",
    description: "Every channel Loomarr manages, and what's on right now.",
    actions: <Action>Add a channel</Action>,
  },
};

export default meta;
export { TitleOnly, WithActions, WithDescription };
