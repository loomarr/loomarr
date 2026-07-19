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
    displayable: true,
  },
  {
    name: "session_secret",
    label: "Session secret",
    purpose: "Signs session cookies. Nothing to paste anywhere, so it is never displayed.",
    consequence: "Every session is revoked, including yours.",
    displayable: false,
  },
];

// §4's display policy, differentiated by purpose: values you must paste elsewhere are
// viewable on demand; the one with nothing to paste never is.
const meta = {
  title: "Loomarr/SecretsPanel",
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
