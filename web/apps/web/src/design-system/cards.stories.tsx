import { Icon, icons, Text } from "@loomarr/design-system";
import {
  CardAction,
  CardStatus,
  ChannelChip,
  ListGroup,
  PictureCard,
  type PictureRatio,
  RowCard,
} from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The two card families (#2765, Direction mock "Two families"): a picture card for things with a
// picture (programmes, titles, clips, requests) and a row card for things without one (channels,
// sources, people, settings). Same words and the same one action in both.
const meta = {
  title: "Loomarr Components/Cards",
  component: PictureCard,
  args: { media: null, meta: [], orientation: "top", ratio: "16/9", title: "A title" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 1200, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof PictureCard>;

type Story = StoryObj<typeof meta>;

// A code-drawn stand-in picture: a gradient from the design's own hues, so the story needs no image.
const Still = ({ hue }: { hue: number }) => (
  <div
    aria-hidden
    style={{
      background: `linear-gradient(135deg, hsl(${hue} 35% 22%), hsl(${(hue + 40) % 360} 45% 12%))`,
      height: "100%",
      width: "100%",
    }}
  />
);

const grid = (columns: number) => ({
  display: "grid",
  gap: 12,
  gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
});

const Programmes: Story = {
  render: () => (
    <div style={grid(3)}>
      <PictureCard
        action={<CardAction>Watch live</CardAction>}
        chip={<ChannelChip name="Late Night Sci-Fi" number="03" />}
        media={<Still hue={220} />}
        meta={[
          "Season 2, episode 5",
          <Text key="t" textRole="cardTime" tone="secondary">
            9:00 – 9:30 PM
          </Text>,
        ]}
        orientation="top"
        ratio="16/9"
        title="The Lantern Station"
      />
      <PictureCard
        action={<CardAction>Watch live</CardAction>}
        chip={<ChannelChip name="Classic Movies" number="12" />}
        media={<Still hue={160} />}
        meta={[
          "Film",
          <Text key="t" textRole="cardTime" tone="secondary">
            8:30 – 10:45 PM
          </Text>,
        ]}
        orientation="top"
        ratio="16/9"
        title="Harbor Lights"
      />
    </div>
  ),
};

const Requests: Story = {
  render: () => (
    <ListGroup aria-label="Requests">
      <PictureCard
        action={<CardAction attention>Review</CardAction>}
        chip={<ChannelChip name="Late Night Sci-Fi" number="03" />}
        media={<Still hue={260} />}
        meta={[<CardStatus key="s" label="Waiting for approval" tone="attention" />, "Requested 2 days ago"]}
        orientation="left"
        ratio="2/3"
        title="The Copper Hills"
      />
      <PictureCard
        action={<CardAction>Open</CardAction>}
        media={<Still hue={120} />}
        meta={[<CardStatus key="s" label="Ready to watch" tone="ok" />, "Requested 1 week ago"]}
        orientation="left"
        ratio="2/3"
        title="A Quiet Harvest"
      />
    </ListGroup>
  ),
};

const Clips: Story = {
  render: () => (
    <div style={grid(4)}>
      {[
        { corner: "0:30", hue: 15, status: undefined, title: "Fizzwell Soda: summer jingle" },
        { corner: "0:30", hue: 200, status: "Failed its quality check", title: "Cloudy Day Soup: jingle" },
        { corner: "0:10", hue: 80, status: undefined, title: "Station break: five to nine" },
      ].map((clip) => (
        <PictureCard
          action={<CardAction attention={clip.status !== undefined}>Preview</CardAction>}
          corner={clip.corner}
          key={clip.title}
          media={<Still hue={clip.hue} />}
          meta={[
            ...(clip.status ? [<CardStatus key="s" label={clip.status} tone="error" />] : []),
            "Commercial · 1990s",
            "Kids · aired 12 times",
          ]}
          orientation="top"
          ratio="1/1"
          title={clip.title}
        />
      ))}
    </div>
  ),
};

// A phone: one column of programmes, two of clips.
const Phone: Story = {
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 390 }}>
        <Story />
      </div>
    ),
  ],
  render: () => (
    <div style={{ display: "grid", gap: 12 }}>
      <PictureCard
        action={<CardAction>Watch live</CardAction>}
        chip={<ChannelChip name="Saturday Cartoons" number="07" />}
        media={<Still hue={30} />}
        meta={["Episode 41", "9:00 – 9:30 PM"]}
        orientation="top"
        ratio="16/9"
        title="Captain Pogo’s Clubhouse"
      />
      <div style={grid(2)}>
        <PictureCard
          action={<CardAction>Preview</CardAction>}
          corner="0:15"
          media={<Still hue={300} />}
          meta={["Bumper · 1990s", "Kids · aired 9 times"]}
          orientation="top"
          ratio="1/1"
          title="Back after the break"
        />
        <PictureCard
          action={<CardAction>Preview</CardAction>}
          corner="0:45"
          media={<Still hue={50} />}
          meta={["Commercial · 1980s", "General · aired 2 times"]}
          orientation="top"
          ratio="1/1"
          title="Moonpie Motors: dealership spot"
        />
      </div>
    </div>
  ),
};

const Rows: Story = {
  render: () => (
    <ListGroup aria-label="Sources">
      <RowCard
        action={<CardAction>Check now</CardAction>}
        lead={<Icon decorative glyph={icons.folder} size="default" tone="muted" />}
        meta={[<CardStatus key="s" label="Ready" tone="ok" />, "42 clips · checked 2 hours ago"]}
        title="Drop folder"
      />
      <RowCard
        action={<CardAction attention>Try again</CardAction>}
        lead={<Icon decorative glyph={icons.archive} size="default" tone="muted" />}
        meta={[
          <CardStatus key="s" label="Hasn’t checked in for 23 hours" tone="warn" />,
          "18 clips · last checked 23 hours ago",
        ]}
        title="Local news breaks"
      />
    </ListGroup>
  ),
};

const People: Story = {
  render: () => (
    <ListGroup aria-label="People">
      <RowCard
        action={<CardAction>Manage</CardAction>}
        lead={<Text textRole="cardInitials">A</Text>}
        meta={[<CardStatus key="s" label="You" tone="ok" />, "Admin", "Media-server account · seen today"]}
        title="Ada Marlowe"
      />
      <RowCard
        action={<CardAction attention>New invite</CardAction>}
        lead={<Text textRole="cardInitials">J</Text>}
        meta={[<CardStatus key="s" label="Invitation expired" tone="warn" />, "Member", "Invited 9 days ago"]}
        title="Juniper Oake"
      />
    </ListGroup>
  ),
};

// One list per ratio so the fixed media widths can be compared.
const Ratios: Story = {
  render: () => (
    <div style={{ display: "grid", gap: 12 }}>
      {(["16/9", "2/3", "1/1"] as PictureRatio[]).map((ratio) => (
        <ListGroup aria-label={`Ratio ${ratio}`} key={ratio}>
          <PictureCard
            action={<CardAction>Open</CardAction>}
            media={<Still hue={190} />}
            meta={[`Ratio ${ratio}`]}
            orientation="left"
            ratio={ratio}
            title="A title long enough to need more than one line when the row gets narrow"
          />
        </ListGroup>
      ))}
    </div>
  ),
};

export default meta;
export { Clips, People, Phone, Programmes, Ratios, Requests, Rows };
