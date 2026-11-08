import { Badge, Text } from "@loomarr/design-system";
import { ListGroup, PanelRow } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// PanelRow — the dashboard panels' row (Playout, Services, Activity): an identity region and a
// trailing cluster that drops to its own line rather than being clipped.
const meta = {
  title: "Loomarr Components/PanelRow",
  component: PanelRow,
  args: { children: null },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 672, padding: 24 }}>
        <ListGroup>
          <Story />
        </ListGroup>
      </div>
    ),
  ],
} satisfies Meta<typeof PanelRow>;

type Story = StoryObj<typeof meta>;

const badges = (
  <>
    <Text textRole="captionMono">1.7× rt</Text>
    <Badge shape="square" tone="lock">
      Hardware
    </Badge>
    <Badge shape="square" tone="lock">
      ok
    </Badge>
  </>
);

// A typical row: identity that truncates and a meta cluster that wraps to its own line when narrow.
const Default: Story = {
  render: () => (
    <PanelRow meta={badges}>
      <Text numberOfLines={1} strong textRole="body">
        #3 · 1980s Action Heroes
      </Text>
      <Text numberOfLines={1} textRole="caption" tone="secondary">
        browser · transcode · 1 viewer
      </Text>
    </PanelRow>
  ),
};

// A long label truncates rather than pushing the meta cluster off the edge.
const LongLabel: Story = {
  render: () => (
    <PanelRow meta={badges}>
      <Text numberOfLines={1} strong textRole="body">
        #12 · A Very Long Channel Name That Would Otherwise Shove The Badges Off The Right Edge
      </Text>
    </PanelRow>
  ),
};

// No meta cluster (Activity): the identity region takes the whole row.
const MainOnly: Story = {
  render: () => (
    <PanelRow>
      <Text textRole="body">Approved “Cozy Mystery Nights” and started building the channel.</Text>
    </PanelRow>
  ),
};

export default meta;
export { Default, LongLabel, MainOnly };
