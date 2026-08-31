import { ApiError } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { CreateLocalDialog } from "./create-local-dialog";

const meta = {
  title: "People/CreateLocalDialog",
  component: CreateLocalDialog,
  args: {
    defaultOpen: true,
    onCreate: () => {},
  },
  parameters: { layout: "fullscreen" },
  render: (args) => (
    <div className="h-screen w-screen p-6">
      <CreateLocalDialog {...args} portalContainer={document.getElementById("storybook-root")} />
    </div>
  ),
} satisfies Meta<typeof CreateLocalDialog>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Closed: Story = { args: { defaultOpen: false } };
const Pending: Story = { args: { creating: true } };
const DuplicateUsername: Story = {
  args: {
    error: new ApiError(409, {
      title: "Username already exists",
      detail: "Choose another username.",
      status: 409,
    }),
  },
};
const Validation: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Create account" }));
    await canvas.findByText("Pick a username.");
  },
};
const AdminSelected: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("combobox", { name: "Role" }));
    await userEvent.keyboard("a{Enter}");
  },
};

export default meta;
export { AdminSelected, Closed, Default, DuplicateUsername, Pending, Validation };
