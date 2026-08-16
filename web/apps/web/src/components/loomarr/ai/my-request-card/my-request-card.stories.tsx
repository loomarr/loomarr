import type { ProposalDTO, ProposalJobDTO } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { MyRequestCard } from "./my-request-card";

const proposal = (over: Partial<ProposalDTO> = {}): ProposalDTO =>
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

const job = (over: Partial<ProposalJobDTO> = {}): ProposalJobDTO => ({
  jobId: "j1",
  status: "done",
  intent: { description: "Saturday morning cartoons for the kids" },
  attempts: 1,
  createdAt: "2026-08-15T12:00:00Z",
  updatedAt: "2026-08-15T12:00:12Z",
  proposal: proposal(),
  ...over,
});

const meta = {
  title: "AI/MyRequestCard",
  component: MyRequestCard,
  decorators: [widthFrame(560)],
} satisfies Meta<typeof MyRequestCard>;

type Story = StoryObj<typeof meta>;

const Queued: Story = { args: { job: job({ status: "queued", attempts: 0, proposal: undefined }) } };
const Running: Story = { args: { job: job({ status: "running", proposal: undefined }) } };
const Failed: Story = {
  args: {
    job: job({
      status: "failed",
      proposal: undefined,
      failure: {
        code: "provider_unavailable",
        message: "The AI provider is unavailable right now. Check the AI connection or try again later.",
      },
    }),
  },
};
const WaitingForApproval: Story = { args: { job: job() } };
const Approved: Story = {
  args: { job: job({ proposal: proposal({ status: "approved", approvedBy: "boss" }) }) },
};
const ApprovedWithChanges: Story = {
  args: {
    job: job({
      proposal: proposal({
        status: "approved",
        approvedBy: "boss",
        modSummary: "dropped 2, added 1",
        note: "swapped Gargoyles for Darkwing Duck — we already have that one",
      }),
    }),
  },
};
const Denied: Story = {
  args: {
    job: job({
      proposal: proposal({
        status: "denied",
        denyReason: "over the acquisition cap this week — ask again Monday",
      }),
    }),
  },
};

export default meta;
export { Approved, ApprovedWithChanges, Denied, Failed, Queued, Running, WaitingForApproval };
