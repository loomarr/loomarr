import { formatGuideTime } from "@loomarr/core/guide";
import { Surface } from "@loomarr/design-system";
import { FilmStrip, ProgrammeFacts, type StripAiring, StripMessage } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// A later programme's facts (frontend-design §9.2, target): on an ink plate over the picture, and on
// the page under the Guide on an upright phone. Genre words only: the repo is public.

const day = Date.UTC(2026, 9, 29);
const at = (h: number, m = 0) => day + (h * 60 + m) * 60_000;
const NOW = at(20, 26);
const formatTime = (ms: number) => formatGuideTime(ms, "UTC");

const film: StripAiring = {
  description: "A small town wakes to find every clock an hour fast, and one person remembers why.",
  genres: ["Science fiction", "Mystery"],
  kind: "program",
  rating: "PG",
  scheduleBlockId: "film",
  startMs: at(21),
  stopMs: at(22, 45),
  title: "A space mystery film",
};
const episode: StripAiring = {
  ...film,
  episode: 4,
  scheduleBlockId: "episode",
  season: 2,
  series: "A police drama",
  title: "The long watch",
};

// A washed-out frame: nearly white, the hardest ground for words.
const WashedOut = ({ children }: { children: React.ReactNode }) => (
  <div style={{ background: "linear-gradient(135deg, #ffffff, #e9e4d8)", height: 360, padding: 32 }}>
    {children}
  </div>
);

const meta = {
  title: "Loomarr Components/Programme Facts",
  component: ProgrammeFacts,
  args: { airing: episode, formatTime, ground: "picture", nowMs: NOW },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ProgrammeFacts>;

type Story = StoryObj<typeof meta>;

const light = { theme: "light" } as const;

const OverWashedOutFrame: Story = {
  render: (args) => (
    <WashedOut>
      <ProgrammeFacts {...args} />
    </WashedOut>
  ),
};
const OverWashedOutFrameLight: Story = { ...OverWashedOutFrame, globals: light };

// An upright phone: the Guide's row, then the facts of the programme looked at under it. A library
// without artwork shows the same facts, since they are type alone.
const underGuide = (withArtwork: boolean): Story => ({
  args: { airing: film, ground: "page" },
  render: (args) => (
    <Surface backgroundColor="surfaceCanvas" borderWidth={0} gap={16} padding={16} width={390}>
      <FilmStrip
        airings={[
          {
            ...episode,
            startMs: at(20),
            stopMs: at(21),
            ...(withArtwork
              ? {
                  thumbUrl:
                    "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='32' height='18'%3E%3Crect width='32' height='18' fill='%23356'/%3E%3C/svg%3E",
                }
              : {}),
          },
          args.airing,
        ]}
        formatTime={formatTime}
        layout="row"
        nowMs={NOW}
        window={{ startMs: at(20), stopMs: at(22) }}
      />
      <ProgrammeFacts {...args} />
    </Surface>
  ),
});
const UnderGuidePhone: Story = underGuide(true);
const UnderGuidePhoneLight: Story = { ...underGuide(true), globals: light };
const UnderGuidePhoneNoArtwork: Story = underGuide(false);
const UnderGuidePhoneNoArtworkLight: Story = { ...underGuide(false), globals: light };

// What OK on a later programme raises on a television.
const Message: Story = {
  render: () => (
    <WashedOut>
      <StripMessage
        message={`A space mystery film starts at ${formatTime(at(21))}. OK on what is on now watches Late Night`}
      />
    </WashedOut>
  ),
};
const MessageLight: Story = { ...Message, globals: light };

export {
  Message,
  MessageLight,
  OverWashedOutFrame,
  OverWashedOutFrameLight,
  UnderGuidePhone,
  UnderGuidePhoneLight,
  UnderGuidePhoneNoArtwork,
  UnderGuidePhoneNoArtworkLight,
};
export default meta;
