import { getPreviewChannelPodsMockHandler } from "@loomarr/api/msw";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ChannelBreaks } from "./channel-breaks";

// What plays BETWEEN the shows, on the channel Info panel (§10, §12).
//
// ⚠ The one surface for `GET /v1/channels/{id}/pods`, which had no caller at all until V51f. It is
// member-readable where the Filler tab's draft sandbox is admin-only, so this is how a viewer sees
// why a channel sounds the way it does.
const meta = {
  title: "Channels/ChannelBreaks",
  component: ChannelBreaks,
  decorators: [widthFrame(520)],
  args: { channelId: "ch-1" },
} satisfies Meta<typeof ChannelBreaks>;

type Story = StoryObj<typeof meta>;

const Default: Story = {
  parameters: {
    msw: {
      handlers: [
        getPreviewChannelPodsMockHandler({
          entries: [
            {
              path: "b1.mp4",
              tunarrProgramId: "b1",
              name: "We'll be right back",
              kind: "bumper",
              durationMs: 5000,
              isFallbackCard: false,
            },
            {
              path: "a1.mp4",
              tunarrProgramId: "a1",
              name: "Frosted Flakes",
              kind: "commercial",
              durationMs: 30000,
              isFallbackCard: false,
              era: 1992,
            },
            {
              path: "a2.mp4",
              tunarrProgramId: "a2",
              name: "TMNT figures",
              kind: "commercial",
              durationMs: 30000,
              isFallbackCard: false,
              era: 1992,
            },
          ],
          totalMs: 65000,
          matchLevel: "exact",
        }),
      ],
    },
  },
};

export default meta;
export { Default };
