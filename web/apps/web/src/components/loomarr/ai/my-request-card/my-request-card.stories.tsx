import type { ProposalDTO } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { MyRequestCard } from "./my-request-card";

const base = (over: Partial<ProposalDTO> = {}): ProposalDTO =>
  ({
    id: "p1",
    jobId: "j1",
    status: "submitted",
    createdBy: "u-kid",
    proposal: {
      intent: { description: "Saturday morning cartoons for the kids" },
      lineup: [
        { name: "DuckTales", mediaType: "series", inLibrary: true },
        { name: "Animaniacs", mediaType: "series", inLibrary: true },
      ],
      acquisitions: [{ name: "Gargoyles", mediaType: "series", inLibrary: false }],
    },
    ...over,
  }) as ProposalDTO;

// One submitted request, from the REQUESTER's side (V26 / `A2`). The four states below are the
// four answers to "what happened to what I asked for?" — and the last two are the ones that
// were previously invisible: the provenance was stored and rendered nowhere.
const meta = {
  title: "AI/MyRequestCard",
  component: MyRequestCard,
  decorators: [widthFrame(560)],
} satisfies Meta<typeof MyRequestCard>;

type Story = StoryObj<typeof meta>;

const Waiting: Story = { args: { proposal: base() } };

const Approved: Story = { args: { proposal: base({ status: "approved", approvedBy: "boss" }) } };

// Approved-with-changes is a DISTINCT outcome: the lineup someone receives is not the one they
// asked for, and "Approved" alone would hide that.
const ApprovedWithChanges: Story = {
  args: {
    proposal: base({
      status: "approved",
      approvedBy: "boss",
      modSummary: "dropped 2, added 1",
      note: "swapped Gargoyles for Darkwing Duck — we already have that one",
    }),
  },
};

// A denial without a reason teaches the requester nothing, so the reason is the point of the
// card in this state.
const Denied: Story = {
  args: {
    proposal: base({ status: "denied", denyReason: "over the acquisition cap this week — ask again Monday" }),
  },
};

export default meta;
export { Approved, ApprovedWithChanges, Denied, Waiting };
