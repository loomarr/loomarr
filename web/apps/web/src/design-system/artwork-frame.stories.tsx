import { ArtworkFrame, type ArtworkRatio, type ArtworkState, Surface } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// ArtworkFrame: the artwork-placeholder box a still, channel or poster sits in. 16:9 is the default
// shape; a film or series poster asks for 2:3. The "art" here is a token fill, not an image.
const Frames = ({ ratio, state, width }: { ratio: ArtworkRatio; state: ArtworkState; width: number }) => (
  <div
    style={{
      display: "grid",
      gap: 12,
      gridTemplateColumns: "repeat(3, minmax(0, 1fr))",
      maxWidth: width,
      padding: 16,
    }}
  >
    {[0, 1, 2].map((index) => (
      <ArtworkFrame borderRadius="control" key={index} ratio={ratio} state={state} width="100%">
        <Surface
          backgroundColor={index === 1 ? "stateInfo" : "surfaceElevated"}
          borderWidth={0}
          height="100%"
          width="100%"
        />
      </ArtworkFrame>
    ))}
  </div>
);

const meta = {
  title: "Loomarr Foundations/ArtworkFrame",
  component: Frames,
  args: { ratio: "16:9", state: "ready", width: 720 },
} satisfies Meta<typeof Frames>;

type Story = StoryObj<typeof meta>;

const Wide: Story = {};
const Poster: Story = { args: { ratio: "2:3" } };
const PosterMissing: Story = { args: { ratio: "2:3", state: "missing" } };
const PosterLoading: Story = { args: { ratio: "2:3", state: "loading" } };
const PosterError: Story = { args: { ratio: "2:3", state: "error" } };
// A phone wall: three posters across 390.
const PosterPhone: Story = { args: { ratio: "2:3", width: 390 } };
const PosterLight: Story = { args: { ratio: "2:3", state: "missing" }, globals: { theme: "light" } };

export default meta;
export { Poster, PosterError, PosterLight, PosterLoading, PosterMissing, PosterPhone, Wide };
