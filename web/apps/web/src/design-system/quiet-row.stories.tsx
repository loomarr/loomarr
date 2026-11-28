import { Surface } from "@loomarr/design-system";
import { QuietRow, type QuietRowProps } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";

// The quiet row of The Cross (frontend-design §9.3, target), per device. A window and an upright
// phone draw it on the page's own ground; elsewhere it brings its own ink over the picture.

const meta = {
  title: "Loomarr Components/Quiet Row",
  component: QuietRow,
  args: { device: "full", onPress: () => undefined },
  render: (args: QuietRowProps) => (
    <Surface
      backgroundColor="surfaceCanvas"
      borderWidth={0}
      justifyContent="flex-end"
      minHeight={220}
      width={args.device === "portrait" ? 390 : 844}
    >
      <QuietRow {...args} />
    </Surface>
  ),
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof QuietRow>;

type Story = StoryObj<typeof meta>;

const light = { theme: "light" } as const;

const FullScreen: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.hover(within(canvasElement).getByRole("button", { name: "Subtitles (C)" }));
    await expect(await within(document.body).findByRole("tooltip")).toHaveTextContent("Subtitles (C)");
  },
};
const FullScreenGroupOpen: Story = { args: { group: "Favourites", listOpen: true } };
const FullScreenPausedMuted: Story = { args: { muted: true, openKey: "audio", paused: true } };
const Window: Story = { args: { device: "window" } };
const WindowLight: Story = { ...Window, globals: light };
const Television: Story = { args: { device: "tv" } };
const TelevisionBehindLive: Story = {
  args: { behindLive: true, device: "tv" },
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await userEvent.keyboard("{ArrowRight}{ArrowRight}{ArrowRight}");
    const canvas = within(canvasElement);
    await expect(canvas.getAllByTestId("ring-actionFocus")).toHaveLength(1);
    await expect(canvas.getByRole("button", { name: "Pause" })).toHaveTextContent("Pause");
  },
};
const Landscape: Story = { args: { device: "landscape" } };
const Portrait: Story = { args: { device: "portrait" } };
const PortraitLight: Story = { ...Portrait, globals: light };
const WindowFocusLight: Story = {
  args: { device: "window" },
  globals: light,
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await expect(within(canvasElement).getAllByTestId("ring-actionFocus")).toHaveLength(1);
  },
};

export {
  FullScreen,
  FullScreenGroupOpen,
  FullScreenPausedMuted,
  Landscape,
  Portrait,
  PortraitLight,
  Television,
  TelevisionBehindLive,
  Window,
  WindowFocusLight,
  WindowLight,
};
export default meta;
