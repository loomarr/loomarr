import { ApiError } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { LoginForm } from "./login-form";

const noop = () => {};

// The sign-in surface (§11): idle · submitting · rejected. A real rejection is the
// BE's single `invalid credentials` problem (no user enumeration), rendered as words.
const meta = {
  title: "Loomarr/LoginForm",
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

export default meta;
export { Idle, Rejected, Submitting };
