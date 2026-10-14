import { SizeConfidence } from "@loomarr/core/requests";
import { Screen } from "@loomarr/design-system";
import {
  ApprovalConsequenceDialog,
  type ApprovalConsequenceDialogProps,
  type ChannelRequestConsequence,
  type FillerDownloadConsequence,
} from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

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
const updatesExisting: ChannelRequestConsequence[] = allKnown.map((item, i) =>
  i === 0 ? { ...item, channel: { kind: "update", name: "Grey Day Mysteries" } } : item,
);

const pulls: FillerDownloadConsequence[] = [
  { id: "fp-1", title: "Station-ident bumpers", estimateClips: 18, sources: 2 },
  { id: "fp-2", title: "Archival weather segments", estimateClips: 9, sources: 1 },
];

const DialogStage = (props: ApprovalConsequenceDialogProps) => (
  <Screen>
    <ApprovalConsequenceDialog {...props} />
  </Screen>
);

const meta = {
  title: "Loomarr Components/Approval Consequence Dialog",
  component: DialogStage,
  args: {
    visible: true,
    consequences: { group: "channels", items: mixed },
    onCancel: () => {},
    onConfirm: () => {},
    onRetry: () => {},
    onDone: () => {},
  },
  parameters: { layout: "fullscreen" },
  tags: ["portal"],
} satisfies Meta<typeof DialogStage>;

type Story = StoryObj<typeof meta>;

// Every size known: "Size: 12.4 GB" is a real total.
const AllKnown: Story = { args: { consequences: { group: "channels", items: allKnown } } };

// Some known: the sum names what it covers — "8.4 GB for 4 of 20 titles · 16 unknown".
const MixedKnownUnavailable: Story = {};

// Nothing known: "Size: estimate unavailable" on every line, never "0 B" and never omitted.
const AllUnavailable: Story = { args: { consequences: { group: "channels", items: allUnavailable } } };

// The job already has a channel, so approving updates it rather than creating one.
const UpdatesExistingChannel: Story = {
  args: { consequences: { group: "channels", items: updatesExisting } },
};

// Filler pulls carry no size and no channel until a pull producer returns (beta.11).
const FillerDownloads: Story = { args: { consequences: { group: "filler", items: pulls } } };

const Approving: Story = { args: { pending: true } };

// The whole call failed, so there are no per-item results to show yet.
const CallFailed: Story = { args: { error: "Couldn't reach Loomarr. Check the connection and try again." } };

// One item failed for good (already decided, a 409): it states why and offers no Retry, and with
// nothing retryable there is no "Retry N failed" either. The rest keep their exact counts.
const PartialFailure: Story = {
  args: {
    results: [
      { id: "cr-1", ok: true, enqueued: 3 },
      { id: "cr-2", ok: false, error: "This suggestion has already been approved or dismissed." },
    ],
  },
};

// Only a failure the server marks retryable (a 5xx) gets its own Retry, and the footer counts only
// those: "Retry 1 failed" here, though two failed.
const RetryableFailure: Story = {
  args: {
    consequences: {
      group: "channels",
      items: [
        ...allKnown,
        {
          id: "cr-3",
          title: "Saturday cartoons",
          titleCount: 2,
          toDownload: 0,
          ceiling: "TV-Y7",
          sizes: [owned(0.6), owned(0.9)],
          channel: create("Saturday cartoons"),
        },
      ],
    },
    results: [
      { id: "cr-1", ok: true, enqueued: 3 },
      { id: "cr-2", ok: false, error: "This suggestion has already been approved or dismissed." },
      { id: "cr-3", ok: false, error: "Couldn't save the approval. Try again.", retryable: true },
    ],
  },
};

// A filler result states the approved pull's exact clip count (#1884), with no estimate language.
// The failed pull couldn't start downloading (a 502), which a retry can clear.
const FillerPartialFailure: Story = {
  args: {
    consequences: { group: "filler", items: pulls },
    results: [
      { id: "fp-1", ok: true, enqueued: 12 },
      {
        id: "fp-2",
        ok: false,
        error: "Loomarr couldn't start downloading. Check the Filler sources and try again.",
        retryable: true,
      },
    ],
  },
};

export default meta;
export {
  AllKnown,
  AllUnavailable,
  Approving,
  CallFailed,
  FillerDownloads,
  FillerPartialFailure,
  MixedKnownUnavailable,
  PartialFailure,
  RetryableFailure,
  UpdatesExistingChannel,
};
