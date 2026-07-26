import type { SessionBody } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { SessionList } from "./session-list";

const noop = () => {};

// A fixed clock so the rendered "2h ago" is stable for the visual suite (§5.2) rather
// than drifting with the wall clock on every snapshot run.
const NOW = Date.parse("2026-07-19T12:00:00Z");
const hoursAgo = (h: number) => NOW - h * 3_600_000;
const hoursAhead = (h: number) => NOW + h * 3_600_000;

const session = (over: Partial<SessionBody> = {}): SessionBody => ({
  id: "hash-a",
  userId: "u1",
  createdAt: hoursAgo(2),
  expiresAt: hoursAhead(48),
  current: false,
  ...over,
});

// Who is signed in as this user, and the ability to end it (§11).
const meta = {
  title: "People/SessionList",
  component: SessionList,
  // The frozen clock the fixtures are relative to, so snapshots do not drift (§5.2).
  args: { userName: "Grace", onRevoke: noop, now: NOW },
  decorators: [widthFrame(640)],
} satisfies Meta<typeof SessionList>;

type Story = StoryObj<typeof meta>;

const Empty: Story = { args: { sessions: [] } };

const Single: Story = { args: { sessions: [session()] } };

const Several: Story = {
  args: {
    sessions: [
      session({ id: "a", createdAt: hoursAgo(2) }),
      session({ id: "b", createdAt: hoursAgo(30), expiresAt: hoursAhead(6) }),
      session({ id: "c", createdAt: hoursAgo(72), expiresAt: hoursAhead(120) }),
    ],
  },
};

// The caller's own session is labelled rather than hidden, so an admin reviewing their
// account cannot sign themselves out by accident.
const IncludingThisDevice: Story = {
  args: { sessions: [session({ id: "a", current: true }), session({ id: "b" })] },
};

const Revoking: Story = {
  args: { sessions: [session({ id: "a" }), session({ id: "b" })], revoking: "a" },
};

const Loading: Story = { args: { sessions: [], loading: true } };

export default meta;
export { Empty, IncludingThisDevice, Loading, Revoking, Several, Single };
