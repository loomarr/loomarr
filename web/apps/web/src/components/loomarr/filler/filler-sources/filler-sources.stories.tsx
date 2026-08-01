import type { FillerSourceDTO } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { FillerSources } from "./filler-sources";

const noop = () => {};

const SOURCES: FillerSourceDTO[] = [
  {
    id: "folder",
    enabled: true,
    switchable: true,
    removable: false,
    kind: "folder",
    target: "/data/filler",
    detail: "watched directly — new files appear on the next pass",
    count: 412,
    configured: true,
    fetchable: true,
  },
  {
    id: "library",
    enabled: true,
    // Not switchable: nothing scans a media-server library for clips since §10 took the media
    // server out of the filler path, so a switch here would change nothing.
    switchable: false,
    removable: false,
    kind: "library",
    target: "media server filler library",
    detail: "scanned by the media server",
    count: 6,
    configured: true,
    fetchable: true,
  },
  {
    id: "remote",
    enabled: true,
    // A container for the registered collections, each of which carries its own switch.
    switchable: false,
    removable: false,
    kind: "remote",
    target: "downloads",
    detail: "fetches clips into the watched folder from a URL you give it",
    count: 0,
    configured: false,
    fetchable: false,
  },
];

const meta = {
  title: "Filler/FillerSources",
  component: FillerSources,
  args: { sources: SOURCES, total: 418, onFetch: noop },
  decorators: [widthFrame(760)],
} satisfies Meta<typeof FillerSources>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

// A fresh install: the folder row is present but unconfigured. Hiding it would leave "why is
// my catalog empty" unanswered — the operator would see two sources and assume filler works.
const NothingConfigured: Story = {
  args: {
    sources: SOURCES.map((s) => ({ ...s, count: 0, configured: false, fetchable: false })),
    total: 0,
  },
};

const Fetching: Story = {
  args: { fetching: "folder" },
};

const FetchFailed: Story = {
  args: { error: "Couldn't reach the media server to re-scan." },
};

export default meta;
export { Default, FetchFailed, Fetching, NothingConfigured };
