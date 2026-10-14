import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge } from "@/components/ui";
import { PanelRow } from "./panel-row";

const meta: Meta<typeof PanelRow> = {
  title: "UI/PanelRow",
  component: PanelRow,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <ul className="max-w-2xl overflow-hidden rounded-md border border-border">
        <Story />
      </ul>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof PanelRow>;

// A typical row: identity that truncates + a meta cluster that wraps to its own line when narrow.
export const Default: Story = {
  render: () => (
    <PanelRow>
      <PanelRow.Main>
        <p className="truncate font-medium text-sm">#3 · 1980s Action Heroes</p>
        <p className="mt-0.5 truncate text-muted-foreground text-xs">browser · transcode · 1 viewer</p>
      </PanelRow.Main>
      <PanelRow.Meta>
        <span className="font-mono text-muted-foreground text-xs">1.7× rt</span>
        <Badge variant="lock">Hardware</Badge>
        <Badge variant="lock">ok</Badge>
      </PanelRow.Meta>
    </PanelRow>
  ),
};

// A long label proves it truncates rather than pushing the meta cluster off the edge.
export const LongLabel: Story = {
  render: () => (
    <PanelRow>
      <PanelRow.Main>
        <p className="truncate font-medium text-sm">
          #12 · A Very Long Channel Name That Would Otherwise Shove The Badges Off The Right Edge
        </p>
      </PanelRow.Main>
      <PanelRow.Meta>
        <Badge variant="caution">Software</Badge>
        <Badge variant="caution">degraded</Badge>
      </PanelRow.Meta>
    </PanelRow>
  ),
};

// No meta cluster (Activity-style): content sits directly in the row.
export const MainOnly: Story = {
  render: () => (
    <PanelRow>
      <span className="w-14 shrink-0 font-mono text-muted-foreground text-xs">2m ago</span>
      <span className="min-w-0 flex-1 text-sm">
        Approved “Cozy Mystery Nights” and started building the channel.
      </span>
    </PanelRow>
  ),
};
