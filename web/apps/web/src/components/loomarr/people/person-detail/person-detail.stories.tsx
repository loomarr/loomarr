import { people } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { PersonDetail } from "./person-detail";

const meta = {
  title: "People/PersonDetail",
  component: PersonDetail,
  args: { user: people.localAdmin, sessions: [], onResetPassword: async () => {} },
  decorators: [widthFrame(576)],
} satisfies Meta<typeof PersonDetail>;

type Story = StoryObj<typeof meta>;
const Local: Story = {};
const Imported: Story = { args: { user: people.offlineReady, onResetPassword: undefined } };
const Disabled: Story = { args: { user: people.disabled } };
const Self: Story = { args: { isSelf: true } };
const Busy: Story = { args: { busy: true } };
const VerifiedContact: Story = {
  args: {
    user: {
      ...people.localAdmin,
      contactAddress: {
        email: "ada@example.com",
        status: "verified",
        provenance: "invitation",
        verifiedAt: 1_900_000_000_000,
      },
    },
    onSetContactAddress: async () => {},
    onRemoveContactAddress: async () => {},
  },
};
const PendingReplacement: Story = {
  args: {
    ...VerifiedContact.args,
    user: {
      ...people.localAdmin,
      contactAddress: {
        email: "ada@example.com",
        status: "verified",
        provenance: "invitation",
        verifiedAt: 1_900_000_000_000,
      },
      contactReplacement: {
        email: "ada.new@example.com",
        status: "pending",
        provenance: "self",
      },
    },
    onCancelContactReplacement: async () => {},
  },
};

export default meta;
export { Busy, Disabled, Imported, Local, PendingReplacement, Self, VerifiedContact };
