import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { within } from "storybook/test";
import { SupportBundle } from "./support-bundle";

const framed = (args: ComponentProps<typeof SupportBundle>) => (
  <div className="h-[680px] w-[min(720px,calc(100vw-32px))]">
    <SupportBundle {...args} />
  </div>
);

const meta = {
  title: "Diagnostics/SupportBundle",
  component: SupportBundle,
  parameters: { layout: "centered" },
} satisfies Meta<typeof SupportBundle>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { correlations: { channelId: "channel-7" } } };
export const Open: Story = {
  args: { correlations: { channelId: "channel-7" } },
  render: framed,
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: /support bundle/i }));
  },
};
export const Previewed: Story = {
  args: { correlations: { channelId: "channel-7" } },
  render: framed,
  play: async ({ canvas, canvasElement, userEvent }) => {
    window.fetch = (() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            estimatedBytes: 24_576,
            manifest: {
              formatVersion: "loomarr.support-bundle.v1",
              generatedAt: 1,
              selection: { from: 1, to: 2, events: true, processes: true, processOutput: true },
              effectiveFrom: 1,
              effectiveTo: 2,
              loomarr: { version: "v0.9.0" },
              clientVersions: ["web:v0.9.0"],
              entries: [
                { name: "system.json", uncompressedBytes: 1024 },
                { name: "events.ndjson", uncompressedBytes: 18_432 },
                { name: "processes/index.json", uncompressedBytes: 5120 },
              ],
              counts: {
                events: 42,
                eventsOmittedAtLeast: 0,
                processes: 3,
                processesOmittedAtLeast: 0,
                processOutputs: 2,
                processOutputsOmitted: 0,
                eventRecorderDrops: 0,
                discardedProcessLines: 7,
                redactions: 5,
              },
              truncationReasons: ["process_output_retention"],
              uncompressedBytes: 24_576,
              finalArchiveBytes: 0,
            },
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        ),
      )) as typeof fetch;
    await userEvent.click(canvas.getByRole("button", { name: /support bundle/i }));
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(page.getByRole("button", { name: /preview contents/i }));
    await page.findByRole("region", { name: /support bundle preview/i });
  },
};
