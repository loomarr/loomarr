import { SizeConfidence } from "@loomarr/api/models/sizeConfidence";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { BulkApproveDialog } from "./bulk-approve-dialog";
import type { ChannelRequestConsequence, FillerDownloadConsequence } from "./bulk-approve-dialog.type";

// What a group's bulk approve will do, before and after (#1817, mock #1876). Every figure is the
// server's; where it has none the dialog says "unavailable" — never "0 B", never a guess.
// Invented fixtures only.

const GiB = 1024 ** 3;
const owned = (gib: number) => ({ sizeBytes: Math.round(gib * GiB), sizeConfidence: SizeConfidence.exact });
const unknown = {};
const create = (name: string) => ({ kind: "create" as const, name });

// Every title owned, so every size is known: the sum is a total.
const allKnown: ChannelRequestConsequence[] = [
  {
    id: "cr-1",
    title: "Rainy afternoon serials",
    titleCount: 3,
    toDownload: 0,
    ceiling: "TV-14",
    sizes: [owned(2.1), owned(1.4), owned(4.9)],
    channel: create("Rainy afternoon serials"),
  },
  {
    id: "cr-2",
    title: "Overnight documentary block",
    titleCount: 2,
    toDownload: 0,
    ceiling: "TV-PG",
    sizes: [owned(3.2), owned(0.8)],
    channel: create("Overnight documentary block"),
  },
];

// Owned movies carry a size; series and titles still to acquire do not (#1885).
const mixed: ChannelRequestConsequence[] = [
  {
    id: "cr-1",
    title: "Rainy afternoon serials",
    titleCount: 14,
    toDownload: 3,
    ceiling: "TV-14",
    sizes: [owned(2.1), owned(2.1), owned(2.1), owned(2.1), ...Array.from({ length: 10 }, () => unknown)],
    channel: create("Rainy afternoon serials"),
  },
  {
    id: "cr-2",
    title: "Overnight documentary block",
    titleCount: 6,
    toDownload: 6,
    ceiling: "TV-PG",
    sizes: Array.from({ length: 6 }, () => unknown),
    channel: create("Overnight documentary block"),
  },
];

const allUnavailable: ChannelRequestConsequence[] = mixed.map((item) => ({
  ...item,
  sizes: item.sizes.map(() => unknown),
}));

// A revision of a request that already has a channel replaces that channel's lineup.
const updatesExisting: ChannelRequestConsequence[] = [
  { ...allKnown[0]!, channel: { kind: "update", name: "Grey Day Mysteries" } },
  allKnown[1]!,
];

const pulls: FillerDownloadConsequence[] = [
  { id: "fp-1", title: "Station-ident bumpers", estimateClips: 18, sources: 2 },
  { id: "fp-2", title: "Archival weather segments", estimateClips: 9, sources: 1 },
];

const meta = {
  title: "AI/BulkApproveDialog",
  component: BulkApproveDialog,
  args: {
    open: true,
    consequences: { group: "channels", items: mixed },
    onCancel: () => {},
    onConfirm: () => {},
    onRetry: () => {},
    onDone: () => {},
  },
  decorators: [widthFrame(760)],
  parameters: { layout: "fullscreen" },
  render: (args) => (
    <div className="h-screen w-screen p-6">
      <BulkApproveDialog {...args} portalContainer={document.getElementById("storybook-root")} />
    </div>
  ),
} satisfies Meta<typeof BulkApproveDialog>;

type Story = StoryObj<typeof meta>;

// Every size known: "Size: 12.4 GB" is a real total.
const AllKnown: Story = { args: { consequences: { group: "channels", items: allKnown } } };

// Some known: the sum names what it covers — "8.4 GB for 4 of 14 titles · 10 unknown".
const MixedKnownUnavailable: Story = {};
const MixedKnownUnavailableMobile: Story = { decorators: [widthFrame(390)] };

// Nothing known: "Size: estimate unavailable" on every line, never "0 B" and never omitted.
const AllUnavailable: Story = { args: { consequences: { group: "channels", items: allUnavailable } } };

// The job already has a channel, so approving updates it rather than creating one.
const UpdatesExistingChannel: Story = {
  args: { consequences: { group: "channels", items: updatesExisting } },
};

// Filler pulls carry no size and no channel until a pull producer returns (beta.11).
const FillerDownloads: Story = { args: { consequences: { group: "filler", items: pulls } } };

const Approving: Story = { args: { pending: true } };

// One item failed: its own Retry, plus "Retry 1 failed". The rest keep their exact counts.
const PartialFailure: Story = {
  args: {
    results: [
      { id: "cr-1", ok: true, enqueued: 3 },
      { id: "cr-2", ok: false, error: "This suggestion has already been approved or dismissed." },
    ],
  },
};
const PartialFailureMobile: Story = { ...PartialFailure, decorators: [widthFrame(390)] };

// A filler result states the approved pull's exact clip count (#1884), with no estimate language.
const FillerPartialFailure: Story = {
  args: {
    consequences: { group: "filler", items: pulls },
    results: [
      { id: "fp-1", ok: true, enqueued: 12 },
      { id: "fp-2", ok: false, error: "This pull has already been approved or dismissed." },
    ],
  },
};

export default meta;
export {
  AllKnown,
  AllUnavailable,
  Approving,
  FillerDownloads,
  FillerPartialFailure,
  MixedKnownUnavailable,
  MixedKnownUnavailableMobile,
  PartialFailure,
  PartialFailureMobile,
  UpdatesExistingChannel,
};
