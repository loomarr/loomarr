import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { RunButton } from "./run-button";

const noop = () => {};

// "Do this now", with visible feedback that it is happening. The busy state is the whole point:
// a queued backend accepts the request in milliseconds, so a button driven by server state
// alone never visibly changes (see useRunFeedback).
const meta = {
  title: "Feedback/RunButton",
  component: RunButton,
  args: { onRun: noop },
  decorators: [widthFrame(240)],
} satisfies Meta<typeof RunButton>;

type Story = StoryObj<typeof meta>;

const Idle: Story = { args: { busy: false } };

// ⚠ Disabled AND aria-busy: "unavailable" and "working" are different claims, and a screen
// reader needs the second one.
const Busy: Story = { args: { busy: true } };

// The labels are overridable for surfaces where "Run now" is the wrong verb.
const CustomLabels: Story = {
  args: { busy: false, label: "Sync now", busyLabel: "Syncing…" },
};

export default meta;
export { Busy, CustomLabels, Idle };
