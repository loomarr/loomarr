import type { LLMModelView } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ModelPicker } from "./model-picker";

const noop = () => {};

const model = (over: Partial<LLMModelView> & Pick<LLMModelView, "tag" | "label">): LLMModelView => ({
  approxVramGiB: 6,
  fit: "fits",
  pulled: true,
  recommended: false,
  runtimeOk: true,
  tools: true,
  why: "Good tool-calling at this size.",
  ...over,
});

const CATALOG: LLMModelView[] = [
  model({
    tag: "qwen3:8b",
    label: "Qwen3 8B",
    recommended: true,
    why: "Best fit for your GPU with reliable tool-calling.",
  }),
  model({
    tag: "llama3.1:8b",
    label: "Llama 3.1 8B",
    pulled: false,
    why: "Solid all-rounder; needs downloading first.",
  }),
  model({
    tag: "qwen3:32b",
    label: "Qwen3 32B",
    approxVramGiB: 22,
    fit: "tight",
    pulled: false,
    why: "Stronger reasoning, but it will swap on this card.",
  }),
  model({
    tag: "llama3.1:70b",
    label: "Llama 3.1 70B",
    approxVramGiB: 40,
    fit: "wont_fit",
    pulled: false,
    why: "Far beyond the detected VRAM.",
  }),
];

// The §8.1 picker: the BE probes the machine and ranks the catalog, so the UI renders a
// judgement rather than making one. Unusable models stay visible-but-disabled, because
// "why isn't it listed?" is a worse question than seeing the reason.
const meta = {
  title: "AI/ModelPicker",
  component: ModelPicker,
  args: {
    catalog: CATALOG,
    active: "qwen3:8b",
    gpuName: "Apple M3 Max",
    vramGiB: 16,
    onSelect: noop,
    onPull: noop,
  },
  decorators: [widthFrame(560)],
} satisfies Meta<typeof ModelPicker>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const NothingSelected: Story = { args: { active: undefined } };
const Downloading: Story = {
  args: { pulling: { tag: "llama3.1:8b", percent: 71 } },
};
const NoGpuDetected: Story = { args: { gpuName: undefined, vramGiB: undefined } };

export default meta;
export { Default, Downloading, NoGpuDetected, NothingSelected };
