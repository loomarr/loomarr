import { drawerChannels } from "@loomarr/fixtures";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, waitFor } from "storybook/test";
import { ChannelDrawer } from "./channel-drawer";

// The drawer sits inside the player's frame, over its right edge. The frame here is a stand-in for
// the picture, 640px tall, so the drawer's 95% canvas shows it faintly.
const inPlayerFrame: Decorator = (Story) => (
  <div className="bg-static-700 relative h-[640px] w-full overflow-hidden">
    <div className="absolute inset-y-0 right-0 z-40 flex max-w-full">
      <Story />
    </div>
  </div>
);

const meta = {
  title: "Channels/ChannelDrawer",
  component: ChannelDrawer,
  decorators: [inPlayerFrame],
  parameters: { layout: "fullscreen" },
  args: {
    channels: drawerChannels,
    favourites: ["ch-saturday"],
    nowPercent: 25,
    onClose: fn(),
    onFavourite: fn(),
    onTune: fn(),
    recent: ["ch-scifi", "ch-thrillers", "ch-sitcoms"],
    tunedId: "ch-thrillers",
  },
} satisfies Meta<typeof ChannelDrawer>;

type Story = StoryObj<typeof meta>;

// Open over several channels: one tuned, one pinned, recents and All channels below. The find box
// has the focus, as it does on every open.
const Open: Story = {
  play: async ({ canvas }) => {
    await waitFor(async () => expect(await canvas.findByRole("searchbox")).toHaveFocus());
  },
};

// A find that matches programmes as well as channel names.
const Matches: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.type(await canvas.findByRole("searchbox"), "sci");
    await canvas.findByText("1 of 6 channels · guide has the full week");
  },
};

// A find that matches nothing: the empty Matches group and its footer.
const NoMatches: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.type(await canvas.findByRole("searchbox"), "zzz");
    await canvas.findByText("0 of 6 channels · guide has the full week");
  },
};

// Keyboard focus on the first row, from the find box.
const RowFocused: Story = {
  play: async ({ canvas, userEvent }) => {
    await waitFor(async () => expect(await canvas.findByRole("searchbox")).toHaveFocus());
    await userEvent.tab();
    await waitFor(() => expect(document.activeElement?.getAttribute("aria-label")).toMatch(/^03 /));
  },
};

export default meta;
export { Matches, NoMatches, Open, RowFocused };
