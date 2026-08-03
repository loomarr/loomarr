import { fillerSources, fillerSourcesWithRemotes } from "@loomarr/fixtures";
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

export default meta;
export { Default, FetchFailed, Fetching, NothingConfigured, WithRegisteredSources };
