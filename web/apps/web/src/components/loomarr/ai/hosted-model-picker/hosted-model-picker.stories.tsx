import type { HostedModelView, HostedProviderView } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { HostedModelPicker } from "./hosted-model-picker";

const noop = () => {};

const FALLBACK_MODEL: HostedModelView = {
  id: "openai/gpt-4o-mini",
  label: "GPT-4o mini",
  why: "Cheap, tool-capable, and a good default for Loomarr's grounded suggestions.",
  recommended: true,
  tools: true,
};

const OPENROUTER = (over: Partial<HostedProviderView> = {}): HostedProviderView => ({
  key: "openrouter",
  label: "OpenRouter",
  baseUrl: "https://openrouter.ai/api/v1",
  keysUrl: "https://openrouter.ai/keys",
  keyConfigured: true,
  active: true,
  modelsLive: true,
  models: [
    {
      id: "openai/gpt-4o-mini",
      label: "GPT-4o mini",
      why: "GPT-4o family — strong grounded tool-caller, ~$0.15/1M tokens",
      recommended: true,
      tools: true,
    },
    {
      id: "anthropic/claude-3.5-haiku",
      label: "Claude 3.5 Haiku",
      why: "Claude Haiku — strong grounded tool-caller, ~$4.80/1M tokens, 200k context",
      tools: true,
    },
    {
      id: "meta-llama/llama-3.3-70b",
      label: "Llama 3.3 70B",
      why: "Llama 3.3 — strong grounded tool-caller, ~$0.12/1M tokens, 128k context",
      tools: true,
    },
  ],
  ...over,
});

const CUSTOM: HostedProviderView = {
  key: "custom",
  label: "Custom endpoint",
  baseUrl: "",
  keysUrl: "",
  keyConfigured: false,
  active: false,
  modelsLive: false,
  models: [],
};

// The §8.1 hosted picker: the BE curates providers and fetches their live model lists;
// this renders them with a recommended pick, and points at where to get a key when none
// is configured.
const meta = {
  title: "AI/HostedModelPicker",
  component: HostedModelPicker,
  args: {
    providers: [OPENROUTER(), CUSTOM],
    activeModel: "openai/gpt-4o-mini",
    onSelect: noop,
  },
  decorators: [widthFrame(560)],
} satisfies Meta<typeof HostedModelPicker>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const NoKeyYet: Story = {
  args: {
    providers: [
      OPENROUTER({
        keyConfigured: false,
        active: false,
        modelsLive: false,
        models: [FALLBACK_MODEL],
      }),
      CUSTOM,
    ],
  },
};
const CuratedFallback: Story = {
  args: { providers: [OPENROUTER({ modelsLive: false, models: [FALLBACK_MODEL] })] },
};

export default meta;
export { CuratedFallback, Default, NoKeyYet };
