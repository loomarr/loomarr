import { ApiError } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ErrorState } from "./error-state";

const noop = () => {};

// The RFC 7807 renderer (§3, §6): a problem's title/detail as words, never raw JSON.
// Retry is offered only when the operation is idempotent.
const meta = {
  title: "Loomarr/ErrorState",
  component: ErrorState,
  decorators: [widthFrame(480)],
} satisfies Meta<typeof ErrorState>;

type Story = StoryObj<typeof meta>;

const Retryable: Story = {
  args: {
    error: new ApiError(502, {
      title: "Seerr is unreachable",
      detail: "Connect timed out after 5s. Check the Seerr URL in Settings → Connections.",
      status: 502,
    }),
    onRetry: noop,
  },
};

const Forbidden: Story = {
  args: {
    error: new ApiError(403, {
      title: "Not allowed",
      detail: "Approving acquisitions needs an admin.",
      status: 403,
    }),
  },
};

export default meta;
export { Forbidden, Retryable };
