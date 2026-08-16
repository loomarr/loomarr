import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ErrorDetails } from "./error-details";

// A failure message that opens up rather than being clipped. Collapsed by default so one
// failing row in a list does not push every other row down the page.
const meta = {
  title: "Feedback/ErrorDetails",
  component: ErrorDetails,
  decorators: [widthFrame(560)],
} satisfies Meta<typeof ErrorDetails>;

type Story = StoryObj<typeof meta>;

const Default: Story = { args: { message: "filler sync: no FILLER_DIR configured" } };

// ⚠ The case the component exists for: a real error is often long and multi-line, and
// clipping it to one row hides the part that identifies the problem.
const LongMultiline: Story = {
  args: {
    message: [
      "library scan: GET http://media.local:8096/Items?Recursive=true: dial tcp 198.51.100.45:8096:",
      "connect: connection refused",
      "",
      "the media server was unreachable for the whole scan window; no titles were confirmed",
    ].join("\n"),
  },
};

// ⚠ There is deliberately NO "renders nothing" story. The gallery harness waits for an element
// before shooting, so a story that renders null times out at 30s — V13 hit this and deleted two
// such stories for the same reason. The empty case is covered by a unit test instead.

export default meta;
export { Default, LongMultiline };
