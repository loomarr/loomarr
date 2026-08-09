import type { Meta, StoryObj } from "@storybook/react-vite";
import { Progress } from "./progress";
import type { ProgressTone } from "./progress.type";

// Progress — the determinate/indeterminate bar (§5.1c). A styled div with `role="progressbar"`,
// never a native <progress>: the native element's fill lives in pseudo-elements Tailwind cannot
// reach, so it draws the browser's colour rather than a token.
const meta = {
  title: "Primitives/Progress",
  component: Progress,
  args: { value: 62, label: "Levelling the sound" },
  parameters: { layout: "padded" },
} satisfies Meta<typeof Progress>;

type Story = StoryObj<typeof meta>;

const Determinate: Story = {};

// ⚠ THE distinction this primitive exists to hold. Omitting `value` announces "busy"; passing 0
// announces "0 percent". Only one of those is true of a task that cannot measure itself — Whisper
// and an LLM turn are single opaque calls, and a bar interpolated over them is invented progress.
//
// Rendered side by side because the difference is invisible in isolation: the 0% bar looks like a
// bar that has not started, which is exactly how the wrong one gets shipped.
const IndeterminateVersusZero: Story = {
  render: () => (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1.5">
        <span className="text-static-400 text-xs">no value — announces "busy", no aria-valuenow</span>
        <Progress label="Listening" />
      </div>
      <div className="flex flex-col gap-1.5">
        <span className="text-static-400 text-xs">value=0 — announces "0 percent", a real measurement</span>
        <Progress value={0} label="Levelling the sound" />
      </div>
    </div>
  ),
};

// ⚠ Typed as Record<ProgressTone, string>, so adding a tone to the union makes THIS fail to
// compile until it is given a caption — a gallery that silently omits the state it was added for
// is the failure mode this pattern is copied from StatusDot to avoid.
const TONE_CAPTIONS: Record<ProgressTone, string> = {
  tune: "in progress (§2.1)",
  signal: "needs a human",
  lock: "done",
  onair: "failed",
};

const AllTones: Story = {
  render: () => (
    <div className="flex flex-col gap-3">
      {(Object.entries(TONE_CAPTIONS) as [ProgressTone, string][]).map(([tone, caption]) => (
        <div key={tone} className="flex flex-col gap-1.5">
          <span className="text-static-400 text-xs">
            {tone} — {caption}
          </span>
          <Progress value={68} label={`${tone} example`} tone={tone} />
        </div>
      ))}
    </div>
  ),
};

export default meta;
export { AllTones, Determinate, IndeterminateVersusZero };
