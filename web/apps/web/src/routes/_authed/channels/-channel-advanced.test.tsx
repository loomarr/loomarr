import type { ChannelDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ChannelAdvanced } from "./-channel-advanced";

// A minimal channel — only the fields ChannelAdvanced reads. `applied` is reconcile-owned
// (§9), so the tests assert it survives a playout edit untouched.
const CHANNEL: ChannelDTO = {
  id: "ch1",
  name: "Saturday Cartoons",
  number: 42,
  status: "live",
  strategy: "shuffle",
  tunarrId: "tun-abc",
  policy: { applied: [{ kind: "blockMax", from: "8", to: "unbounded" }] },
} as ChannelDTO;

describe("ChannelAdvanced — playout backend", () => {
  // The reachability assertion. `policy.playout.backend` was PATCHable and unreachable: §9.1
  // promised a channel "can be moved from its own page" and no page offered the move, so the
  // override existed only for a hand-written policy_json. A control that renders but does not
  // commit is the same defect wearing a nicer coat, so this asserts the committed value.
  it("commits a backend choice through onPolicyChange", async () => {
    const onPolicyChange = vi.fn();
    render(<ChannelAdvanced channel={CHANNEL} onPolicyChange={onPolicyChange} />);

    await userEvent.click(screen.getByLabelText("Streamed by"));
    await userEvent.click(await screen.findByRole("option", { name: "Loomarr" }));

    expect(onPolicyChange).toHaveBeenCalledWith(
      expect.objectContaining({ playout: { backend: "internal" } }),
    );
    // Reconcile-owned `applied` rides through untouched — the edit merges, never rebuilds.
    expect(onPolicyChange.mock.lastCall?.[0].applied).toBe(CHANNEL.policy?.applied);
  });

  // "" is the inherit sentinel (a nil *PlayoutPolicy and an empty Backend mean the same thing
  // in Go). Radix forbids an empty-string item value, so "inherit" is the UI sentinel that must
  // lower back to "" — otherwise the channel would pin itself to whatever the default happened
  // to be at that moment, and §9.1's "changing the default affects new channels only" promise
  // would quietly stop holding for it.
  it("lowers the inherit sentinel back to an empty string", async () => {
    const onPolicyChange = vi.fn();
    render(
      <ChannelAdvanced
        channel={{ ...CHANNEL, policy: { playout: { backend: "tunarr" } } } as ChannelDTO}
        onPolicyChange={onPolicyChange}
      />,
    );

    await userEvent.click(screen.getByLabelText("Streamed by"));
    await userEvent.click(await screen.findByRole("option", { name: "Follow the default" }));

    expect(onPolicyChange).toHaveBeenCalledWith(expect.objectContaining({ playout: { backend: "" } }));
  });

  it("shows the stored override, and the default when there is none", () => {
    const { unmount } = render(
      <ChannelAdvanced
        channel={{ ...CHANNEL, policy: { playout: { backend: "tunarr" } } } as ChannelDTO}
        onPolicyChange={vi.fn()}
      />,
    );
    expect(screen.getByLabelText("Streamed by")).toHaveTextContent("Tunarr");
    unmount();

    render(<ChannelAdvanced channel={CHANNEL} onPolicyChange={vi.fn()} />);
    expect(screen.getByLabelText("Streamed by")).toHaveTextContent("Follow the default");
  });

  // The disclosure is otherwise read-only STATUS shown to admins; without a save handler there
  // is nothing to commit to, so the switch stays hidden rather than rendering inert.
  it("omits the switch when no save handler is supplied", () => {
    render(<ChannelAdvanced channel={CHANNEL} />);
    expect(screen.queryByLabelText("Streamed by")).not.toBeInTheDocument();
    // The rest of the diagnostics still render.
    expect(screen.getByText(/Tunarr channel: tun-abc/)).toBeInTheDocument();
  });
});
