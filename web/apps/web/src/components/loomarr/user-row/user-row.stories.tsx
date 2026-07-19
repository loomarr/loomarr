import type { UserBody } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { UserRow } from "./user-row";

const noop = () => {};

const base: UserBody = {
  id: "u1",
  name: "Grace Hopper",
  role: "member",
  quota: 5,
  disabled: false,
  autoApprove: false,
  local: false,
};

// One row of the §11 allowlist: credential path, role, quota, auto-approve, disabled.
const meta = {
  title: "Loomarr/UserRow",
  component: UserRow,
  args: {
    onRoleChange: noop,
    onQuotaChange: noop,
    onToggleDisabled: noop,
    onToggleAutoApprove: noop,
    onViewSessions: noop,
  },
  decorators: [widthFrame(880)],
} satisfies Meta<typeof UserRow>;

type Story = StoryObj<typeof meta>;

const Imported: Story = { args: { user: base } };

// A local user authenticates against a stored bcrypt hash, so password actions mean
// something for this row and not for an imported one.
const Local: Story = { args: { user: { ...base, name: "Ada", local: true } } };

const Admin: Story = { args: { user: { ...base, role: "admin", autoApprove: true, quota: 0 } } };

const Disabled: Story = { args: { user: { ...base, disabled: true } } };

// Self-demotion and self-disable are refused: there is no in-app undo, and the last
// admin removing their own role locks everyone out.
const Self: Story = { args: { user: { ...base, role: "admin" }, isSelf: true } };

const Busy: Story = { args: { user: base, busy: true } };

export default meta;
export { Admin, Busy, Disabled, Imported, Local, Self };
