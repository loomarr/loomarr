import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { SecretsPanel } from "./secrets-panel";
import type { SecretRow } from "./secrets-panel.type";

const noop = () => {};

const SECRETS: SecretRow[] = [
  {
    name: "api_token",
    label: "API token",
    purpose: "Break-glass admin access for scripts and automation.",
    consequence: "The current token stops working immediately.",
  },
  {
    name: "playout_token",
    label: "Playback token",
    purpose: "Lets a media server read Loomarr's Live TV endpoints.",
    consequence: "Existing tuner and guide URLs stop working immediately.",
  },
];

// §4's display policy: operator-facing credentials are viewable on demand.
const meta = {
  title: "Settings/SecretsPanel",
  component: SecretsPanel,
  args: { secrets: SECRETS, revealed: {}, onReveal: noop, onRegenerate: noop },
  decorators: [widthFrame(560)],
} satisfies Meta<typeof SecretsPanel>;

type Story = StoryObj<typeof meta>;

const Masked: Story = {};
const Revealed: Story = { args: { revealed: { api_token: "lm_9f3c2a7b41d8e05c" } } };
const Regenerating: Story = { args: { busy: "api_token" } };

export default meta;
export { Masked, Regenerating, Revealed };
