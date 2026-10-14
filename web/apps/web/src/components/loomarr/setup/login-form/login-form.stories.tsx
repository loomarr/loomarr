import { ApiError } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { LoginForm } from "./login-form";

const noop = () => {};

// The sign-in surface (§11): idle · submitting · rejected. A real rejection is the
// BE's single `invalid credentials` problem (no user enumeration), rendered as words.
const meta = {
  title: "Setup/LoginForm",
  component: LoginForm,
  args: { onSubmit: noop },
  decorators: [widthFrame(360)],
} satisfies Meta<typeof LoginForm>;

type Story = StoryObj<typeof meta>;

const Idle: Story = {};
const Submitting: Story = { args: { isPending: true } };
const Rejected: Story = {
  args: {
    error: new ApiError(401, {
      title: "Invalid credentials",
      detail: "Check your username and password, then try again.",
    }),
  },
};

// ⚠ SSO sits BELOW the password form on purpose: Loomarr's own sign-in works alongside it,
// never instead of it, so an install whose provider is down is still enterable (§11).
const WithSso: Story = { args: { onSso: () => {} } };

// The refusal an operator will actually hit: the provider was happy, Loomarr's allowlist was
// not. The copy says what to do about it rather than just reporting a failure.
const SsoNoAccount: Story = { args: { onSso: () => {}, ssoError: "sso_no_account" } };

export default meta;
export { Idle, Rejected, SsoNoAccount, Submitting, WithSso };
