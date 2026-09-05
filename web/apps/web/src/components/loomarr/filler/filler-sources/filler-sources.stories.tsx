import {
  fillerSources,
  fillerSourcesEmptyProvider,
  fillerSourcesGrouped,
  fillerSourcesWithRemotes,
} from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { FillerSources } from "./filler-sources";

const noop = () => {};

// ⚠ The rows live in @loomarr/fixtures, not here (frontend-design §5.1b). They are typed
// against the orval-generated DTO, so a contract change breaks the typecheck in ONE place
// rather than at every story that happened to hand-roll the same shape.
const SOURCES = fillerSources;

const meta = {
  title: "Filler/FillerSources",
  component: FillerSources,
  // ⚠ No `total`. This header reads "N of M on"; the catalog size belongs to the page header's
  // `watchLine` pill, not here — two components reporting it is how they start disagreeing.
  args: { sources: SOURCES, onFetch: noop },
  decorators: [widthFrame(760)],
} satisfies Meta<typeof FillerSources>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

// A fresh install: the folder row is present but unconfigured. Hiding it would leave "why is
// my catalog empty" unanswered — the operator would see two sources and assume filler works.
const NothingConfigured: Story = {
  args: {
    sources: SOURCES.map((s) => ({ ...s, count: 0, configured: false, fetchable: false })),
  },
};

const Fetching: Story = {
  args: { fetching: "folder" },
};

const FetchFailed: Story = {
  args: { error: "Couldn't reach the media server to re-scan." },
};

// The flat list with registered sources as PEERS (V37) — an archive collection and a YouTube
// playlist alongside the two config-backed rows. This is the shape the redesigned Sources tab
// actually draws, and the stories above deliberately do NOT cover it: they render a fresh
// install, which has no registered sources at all.
//
// ⚠ It also carries the two states that only exist on a peer row: a fetch time ("never fetched"
// on the playlist) and the switched-off badge, which must read as "stopped fetching" rather
// than "gone" — the clips it already brought in are still in the catalog.
// ⚠ `onToggleEnabled` is passed HERE and in no earlier story, which means the on/off switch —
// the tab's most consequential control, whose copy is a behaviour claim ("Loomarr stops
// scanning, searching and downloading from it") — had never appeared in a visual baseline. A
// pre-existing gap rather than a V37 one, closed here because this is the story that renders
// every row type at once.
const WithRegisteredSources: Story = {
  args: { sources: fillerSourcesWithRemotes, onRemove: noop, onToggleEnabled: noop },
};

// The PROVIDER ROLL-UP (§10 V51c) — one Archive.org row and one YouTube row, each twirling down
// to the collections beneath it.
//
// ⚠ **This shape has been on the wire since PR #201 and no story rendered it**, so the visual
// suite has been green over a flat list the server stopped sending. That is the same class as the
// untyped story stubs in #281: a baseline that agrees with itself about a shape nothing serves.
//
// It carries the two states that only exist on a group, which is why it is one story and not two:
// Archive.org is HALF-running (one collection on, one off — the case a single boolean cannot say,
// and why the group has no switch), while YouTube is fully DORMANT, which used to render as if it
// were running because every off-state was gated on `switchable && !enabled` and a group is not
// switchable.
const GroupedByProvider: Story = {
  args: { sources: fillerSourcesGrouped, onRemove: noop, onToggleEnabled: noop },
};

// A provider with nothing under it — an INVITATION, not a fault. It used to draw the same red
// `not configured` caution Badge a broken drop-folder gets, telling an operator something is
// wrong when nothing is.
const EmptyProvider: Story = {
  args: { sources: fillerSourcesEmptyProvider, onRemove: noop, onToggleEnabled: noop },
};

export default meta;
export {
  Default,
  EmptyProvider,
  FetchFailed,
  Fetching,
  GroupedByProvider,
  NothingConfigured,
  WithRegisteredSources,
};
