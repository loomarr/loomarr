import { MediaTile, Stack, Surface, Text, Tooltip } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// MediaTile — a picture that is also a button: a thumbnail that opens its preview, a poster that is
// picked, a still on a timeline. The pictures here are token fills, so a baseline never depends on an
// image.
const Picture = ({ label }: { label: string }) => (
  <Surface
    alignItems="center"
    backgroundColor="$surfaceElevated"
    borderWidth={0}
    flex={1}
    height="100%"
    justifyContent="center"
    width="100%"
  >
    {/* A visible caption is always part of the tile's name (WCAG 2.5.3), as `label` carries it. */}
    <Text textRole="hint" tone="secondary">
      {label}
    </Text>
  </Surface>
);

const Workshop = () => (
  <Stack gap="section" padding="section">
    <Stack gap="inline">
      <Text textRole="title">Thumbnail, poster, current</Text>
      <Stack align="center" direction="row" gap="control">
        <MediaTile height={30} label="Preview Cereal ad, 1989" onPress={() => undefined} width={54}>
          <Picture label="" />
        </MediaTile>
        <MediaTile height={64} label="Preview Toy commercial, 1992" onPress={() => undefined} width={112}>
          <Picture label="1992" />
        </MediaTile>
        <MediaTile
          aspectRatio={2 / 3}
          label="Use Harbor Lights as the channel icon"
          onPress={() => undefined}
          radius="medium"
          width={96}
        >
          <Picture label="Harbor Lights" />
        </MediaTile>
        <MediaTile current height={64} label="Preview Station ident" onPress={() => undefined} width={112}>
          <Picture label="Station ident" />
        </MediaTile>
        <MediaTile disabled height={64} label="Preview Unchecked clip" width={112}>
          <Picture label="Unchecked clip" />
        </MediaTile>
        <MediaTile
          height={64}
          label="Preview Unsplittable reel"
          onPress={() => undefined}
          radius="medium"
          tone="warning"
          width={112}
        >
          <Picture label="Unsplittable reel" />
        </MediaTile>
      </Stack>
    </Stack>
    <Stack gap="inline">
      <Text textRole="title">As a Tooltip trigger</Text>
      <Tooltip content="Click to play this exact clip." side="bottom">
        <MediaTile height={64} label="Preview Cereal ad, 1989" onPress={() => undefined} width={112}>
          <Picture label="Cereal ad, 1989" />
        </MediaTile>
      </Tooltip>
    </Stack>
  </Stack>
);

const meta = {
  title: "Loomarr Components/MediaTile",
  component: Workshop,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Workshop>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Light: Story = { globals: { theme: "light" } };
// The one amber ring, outside the edge, as every control draws it.
const Focused: Story = {
  play: async ({ canvas }) => {
    canvas.getByRole("button", { name: "Preview Toy commercial, 1992" }).focus();
  },
};
const FocusedLight: Story = {
  globals: { theme: "light" },
  play: async ({ canvas }) => {
    canvas.getByRole("button", { name: "Preview Toy commercial, 1992" }).focus();
  },
};
// A current tile that is also focused keeps its neutral ring inside the amber one.
const CurrentFocused: Story = {
  play: async ({ canvas }) => {
    canvas.getByRole("button", { name: "Preview Station ident" }).focus();
  },
};

export default meta;
export { CurrentFocused, Default, Focused, FocusedLight, Light };
