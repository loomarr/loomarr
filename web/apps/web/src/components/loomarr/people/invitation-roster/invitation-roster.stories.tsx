import type { InvitationBody } from "@loomarr/api/models/invitationBody";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { InvitationRoster } from "./invitation-roster";

const base = {
  id: "invitation-ada",
  kind: "local",
  username: "Ada",
  role: "member",
  status: "pending",
  createdAt: Date.UTC(2030, 0, 1),
  expiresAt: Date.UTC(2030, 0, 8),
  contactAddress: { email: "ada@example.com", status: "pending", provenance: "admin" },
} satisfies InvitationBody;

const invitations: InvitationBody[] = [
  { ...base, emailDelivery: { status: "queued", attemptNumber: 1, updatedAt: Date.UTC(2030, 0, 1) } },
  {
    ...base,
    id: "invitation-grace",
    kind: "library",
    username: undefined,
    libraryUserId: "library-grace",
    displayName: "Grace Hopper",
    role: "admin",
    emailDelivery: { status: "delivered", attemptNumber: 1, updatedAt: Date.UTC(2030, 0, 2) },
  },
  {
    ...base,
    id: "invitation-dorothy",
    username: "Dorothy",
    emailDelivery: {
      status: "failed",
      outcome: "recipient_rejected",
      attemptNumber: 1,
      updatedAt: Date.UTC(2030, 0, 2),
    },
  },
];

const meta = {
  title: "People/InvitationRoster",
  component: InvitationRoster,
  args: { invitations, onSendEmail: () => {}, onShare: () => {} },
  decorators: [widthFrame(960)],
} satisfies Meta<typeof InvitationRoster>;

type Story = StoryObj<typeof meta>;
const DeliveryStates: Story = {};
const Mobile: Story = { decorators: [widthFrame(390)] };
const Empty: Story = { args: { invitations: [] } };
const Loading: Story = { args: { invitations: undefined } };

export default meta;
export { DeliveryStates, Empty, Loading, Mobile };
