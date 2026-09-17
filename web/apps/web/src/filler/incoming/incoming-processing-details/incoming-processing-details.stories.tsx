import type { IncomingProcessingDTO } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { IncomingProcessingDetails } from "./incoming-processing-details";

const at = new Date().toISOString();
const frame = (processing: IncomingProcessingDTO) => (
  <div className="w-full max-w-md">
    <IncomingProcessingDetails processing={processing} />
  </div>
);

const meta = {
  title: "Filler/Incoming/Processing details",
  component: IncomingProcessingDetails,
  args: { processing: { stages: [] } },
} satisfies Meta<typeof IncomingProcessingDetails>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ActiveMeasured: Story = {
  render: () =>
    frame({
      stages: [
        {
          label: "Checking video",
          outcome: "finished",
          outcomeLabel: "Finished",
          note: "This step finished successfully.",
          at,
        },
        {
          label: "Adding details",
          outcome: "in_progress",
          outcomeLabel: "In progress",
          note: "Loomarr is working on this step now.",
          at,
          progress: 47,
        },
      ],
    }),
};

export const ActiveUnmeasured: Story = {
  render: () =>
    frame({
      stages: [
        {
          label: "Adding details",
          outcome: "in_progress",
          outcomeLabel: "In progress",
          note: "Loomarr is working on this step now.",
          at,
        },
      ],
    }),
};

export const SkippedAndRetrying: Story = {
  render: () =>
    frame({
      attempts: 2,
      nextTryAt: new Date(Date.now() + 60_000).toISOString(),
      diagnosticsHref: "/filler/manage#diagnostics",
      stages: [
        {
          label: "Checking video",
          outcome: "finished",
          outcomeLabel: "Finished",
          note: "This step finished successfully.",
          at,
        },
        {
          label: "Checking for separate clips",
          outcome: "not_needed",
          outcomeLabel: "Not needed",
          note: "This step was not needed for this clip.",
          at,
        },
        {
          label: "Adding details",
          outcome: "retrying",
          outcomeLabel: "Trying again",
          note: "This step did not finish. Loomarr will try again automatically.",
          at,
        },
      ],
    }),
};

export const Ready: Story = {
  render: () =>
    frame({
      stages: [
        {
          label: "Checking video",
          outcome: "finished",
          outcomeLabel: "Finished",
          note: "This step finished successfully.",
          at,
        },
        {
          label: "Finishing",
          outcome: "finished",
          outcomeLabel: "Finished",
          note: "This step finished successfully.",
          at,
        },
      ],
    }),
};
