import type { DiscoverModelView } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ModelDiscover } from "./model-discover";

const noop = () => {};

const RESULTS: DiscoverModelView[] = [
  {
    id: "deepreinforce-ai/Ornith-1.0-9B-GGUF",
    label: "Ornith 1.0 9B",
    quant: "Q8_0",
    pullRef: "hf.co/deepreinforce-ai/Ornith-1.0-9B-GGUF:Q8_0",
    sizeGiB: 8.9,
    fit: "fits",
    downloads: 2_472_061,
  },
  {
    id: "unsloth/Qwen3.5-4B-GGUF",
    label: "Qwen3.5 4B",
    quant: "BF16",
    pullRef: "hf.co/unsloth/Qwen3.5-4B-GGUF:BF16",
    sizeGiB: 7.8,
    fit: "fits",
    downloads: 1_089_613,
  },
  {
    id: "unsloth/Qwen3.6-27B-GGUF",
    label: "Qwen3.6 27B",
    quant: "Q3_K_S",
    pullRef: "hf.co/unsloth/Qwen3.6-27B-GGUF:Q3_K_S",
    sizeGiB: 11.5,
    fit: "tight",
    downloads: 754_172,
  },
];

// The §8.1 download surface: the BE sizes popular HF GGUF repos against this machine's
// VRAM and returns only compatible ones, ranked best-first. The UI renders that list —
// no search. Fit/size/quant come from the chosen best-fitting quant.
const meta = {
  title: "Loomarr/ModelDiscover",
  component: ModelDiscover,
  args: {
    results: RESULTS,
    onPull: noop,
  },
  decorators: [widthFrame(560)],
} satisfies Meta<typeof ModelDiscover>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Loading: Story = { args: { results: undefined, loading: true } };
const Downloading: Story = {
  args: { pulling: { tag: "hf.co/deepreinforce-ai/Ornith-1.0-9B-GGUF:Q8_0", percent: 43 } },
};
const NoneCompatible: Story = { args: { results: [] } };
const SourceUnreachable: Story = { args: { results: [], sourceError: true } };

export default meta;
export { Default, Downloading, Loading, NoneCompatible, SourceUnreachable };
