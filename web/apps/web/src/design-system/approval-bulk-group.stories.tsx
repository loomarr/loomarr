import { Screen, Text } from "@loomarr/design-system";
import { ApprovalBulkGroup, type ApprovalBulkGroupProps, type ApprovalBulkRow } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

// One kind of pending decision with its own bulk scope (Decision R1, mock #1876). The web queue
// fills each row with its rich request or pull card; these rows stand in for them.
// Invented fixtures only.

const row = (id: string, title: string, detail: string, excluded = false): ApprovalBulkRow => ({
  id,
  label: title,
  excluded,
  content: (
    <>
      <Text textRole="label">{title}</Text>
      <Text textRole="metadata" tone="secondary">
        {detail}
      </Text>
    </>
  ),
});

const channelRows = [
  row("cr-1", "Rainy afternoon serials", "14 titles · 3 to download · up to TV-14"),
  row("cr-2", "Overnight documentary block", "6 titles · 6 to download · up to TV-PG"),
  row("cr-3", "Weekend cartoon hour", "9 titles · 0 to download · up to TV-Y7", true),
];

// Selection is the group's whole job, so the story owns it the way the queue does.
const GroupStage = (args: ApprovalBulkGroupProps) => {
  const [selected, setSelected] = useState<Set<string>>(new Set(args.selected));
  return (
    <Screen>
      <ApprovalBulkGroup {...args} onSelectedChange={setSelected} selected={selected} />
    </Screen>
  );
};

const meta = {
  title: "Loomarr Components/Approval Bulk Group",
  component: GroupStage,
  args: {
    title: "Channel requests",
    rows: channelRows,
    selected: new Set<string>(),
    onSelectedChange: () => {},
    onApprove: () => {},
  },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof GroupStage>;

type Story = StoryObj<typeof meta>;

// Nothing selected: Approve stays disabled and says so, not just dimmed.
const NoneSelected: Story = {};

// Two of the bulkable rows selected; the edited row has no checkbox and says why.
const SomeSelected: Story = { args: { selected: new Set(["cr-1", "cr-2"]) } };

const FillerDownloads: Story = {
  args: {
    title: "Filler downloads",
    requestedBy: "Requested by Loomarr",
    rows: [
      row("fp-1", "Station-ident bumpers", "~18 clips · 2 sources · estimate, not exact"),
      row("fp-2", "Archival weather segments", "~9 clips · 1 source · estimate, not exact"),
    ],
    selected: new Set(["fp-2"]),
  },
};

export default meta;
export { FillerDownloads, NoneSelected, SomeSelected };
