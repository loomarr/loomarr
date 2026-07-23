import type { DiscoverModelView } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ModelDiscover } from "./model-discover";

const noop = () => {};

const RESULTS: DiscoverModelView[] = [
  {
    id: "unsloth/Qwen3-8B-GGUF",
    label: "Qwen3 8B",
    quant: "Q4_K_M",
    pullRef: "hf.co/unsloth/Qwen3-8B-GGUF",
    sizeGiB: 4.9,
    fit: "fits",
    downloads: 1_089_613,
    role: "balanced",
    recommended: true,
    note: "Best all-round pick — fits comfortably",
  },
  {
    id: "unsloth/Llama-3.1-8B-Instruct-GGUF",
    label: "Llama 3.1 8B",
    quant: "Q4_K_M",
    pullRef: "hf.co/unsloth/Llama-3.1-8B-Instruct-GGUF",
    sizeGiB: 4.6,
    fit: "fits",
    downloads: 754_172,
    role: "balanced",
    recommended: false,
    note: "Best all-round pick — fits comfortably",
  },
  {
    id: "unsloth/gemma-3-12b-it-GGUF",
    label: "Gemma 3 12B",
    quant: "Q4_K_M",
    pullRef: "hf.co/unsloth/gemma-3-12b-it-GGUF",
    sizeGiB: 6.6,
    fit: "fits",
    downloads: 623_000,
    role: "higher_quality",
    recommended: false,
    note: "Higher quality — fits comfortably",
  },
  {
    id: "unsloth/Qwen3-4B-GGUF",
    label: "Qwen3 4B",
    quant: "Q4_K_M",
    pullRef: "hf.co/unsloth/Qwen3-4B-GGUF",
    sizeGiB: 2.3,
    fit: "fits",
    downloads: 297_600,
    role: "faster",
    recommended: false,
    note: "Faster and lighter — fits comfortably",
  },
];

// A long list (> the alternatives cap) to exercise the "show more" collapse.
const MANY: DiscoverModelView[] = Array.from({ length: 11 }, (_, i) => ({
  id: `unsloth/Model-${i}-GGUF`,
  label: `Model ${i}`,
  quant: "Q4_K_M",
  pullRef: `hf.co/unsloth/Model-${i}-GGUF`,
  sizeGiB: 3 + i * 0.4,
  fit: i < 8 ? "fits" : "tight",
  downloads: 900_000 - i * 40_000,
  role: i === 0 ? "balanced" : "faster",
  recommended: i === 0,
  note: i === 0 ? "Best all-round pick — fits comfortably" : "Faster and lighter — fits comfortably",
}));

// The §8.1 download surface: the BE ranks compatible HF GGUF repos FOR LOOMARR (fit →
// reliability → size, popularity only as a tiebreak) and flags one recommended pick. The
// UI leads with that pick as a hero, then a few alternatives, then "show more". No HF
// jargon (quant, downloads) reaches the rows.
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
  args: { pulling: { tag: "hf.co/unsloth/Qwen3-8B-GGUF", percent: 43 } },
};
const NoneCompatible: Story = { args: { results: [] } };
const SourceUnreachable: Story = { args: { results: [], sourceError: true } };
const ManyWithShowMore: Story = { args: { results: MANY } };

export default meta;
export { Default, Downloading, Loading, ManyWithShowMore, NoneCompatible, SourceUnreachable };
