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

export default meta;
export { TitleOnly, WithDescription };
