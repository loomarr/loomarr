import type { ChannelDTO } from "@loomarr/api";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { ChannelAdvanced } from "./-channel-advanced";

// A minimal channel — only the fields ChannelAdvanced reads. Its policy carries an
// `operatorSet` the tests assert survives a playout edit untouched, and its findings a
// relaxation the "Rules loosened" row renders.
const CHANNEL: ChannelDTO = {
  revision: 1,
  id: "ch1",
  name: "Saturday Cartoons",
  number: 42,
  status: "live",
  strategy: "shuffle",
  tunarrId: "tun-abc",
  policy: { operatorSet: ["ordering"] },
  findings: { applied: [{ kind: "blockMax", from: "8", to: "unbounded" }] },
} as ChannelDTO;

describe("ChannelAdvanced — playout backend", () => {
  // The reachability assertion. `policy.playout.backend` was PATCHable and unreachable: §9.1
  // promised a channel "can be moved from its own page" and no page offered the move, so the
  // override existed only for a hand-written policy_json. A control that renders but does not
  // commit is the same defect wearing a nicer coat, so this asserts the committed value.
  it("commits a backend choice through onPolicyChange", async () => {
    const onPolicyChange = vi.fn();
    render(<ChannelAdvanced channel={CHANNEL} onPolicyChange={onPolicyChange} />);

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Streamed by" }), "Loomarr");

    expect(onPolicyChange).toHaveBeenCalledWith(
      expect.objectContaining({ playout: { backend: "internal" } }),
    );
    // The rest of the policy rides through untouched — the edit merges, never rebuilds.
    expect(onPolicyChange.mock.lastCall?.[0].operatorSet).toBe(CHANNEL.policy?.operatorSet);
  });

  // "" is the inherit sentinel (a nil *PlayoutPolicy and an empty Backend mean the same thing
  // in Go). The select's options carry "inherit" rather than "" for it, and that sentinel must
  // lower back to "" — otherwise the channel would pin itself to whatever the default happened
  // to be at that moment. Lowering it back to "" keeps the channel following later changes to
  // the fleet-wide default.
  it("lowers the inherit sentinel back to an empty string", async () => {
    const onPolicyChange = vi.fn();
    render(
      <ChannelAdvanced
        channel={{ ...CHANNEL, policy: { playout: { backend: "tunarr" } } } as ChannelDTO}
        onPolicyChange={onPolicyChange}
      />,
    );

    await userEvent.selectOptions(
      screen.getByRole("combobox", { name: "Streamed by" }),
      "Follow the default",
    );

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
    expect(screen.getByText("Tunarr channel")).toBeInTheDocument();
    expect(screen.getByText("tun-abc")).toBeInTheDocument();
  });

  it("says Tunarr isn't used when Loomarr plays the channel itself", () => {
    render(<ChannelAdvanced channel={{ ...CHANNEL, tunarrId: undefined, inAppPlayable: true }} />);
    expect(screen.getByText("Not used — Loomarr plays this channel itself")).toBeInTheDocument();
  });
});

describe("ChannelAdvanced — rules loosened", () => {
  // The relaxations are the last reconcile's findings, not the operator's policy.
  it("lists the relaxations the channel's findings record", () => {
    render(<ChannelAdvanced channel={CHANNEL} />);
    expect(screen.getByText("No cap on how many episodes run back-to-back (was 8).")).toBeInTheDocument();
    expect(
      screen.queryByText("Running exactly as specified. Nothing had to be eased."),
    ).not.toBeInTheDocument();
  });

  it("says nothing was eased when the findings record no relaxation", () => {
    render(<ChannelAdvanced channel={{ ...CHANNEL, findings: {} }} />);
    expect(screen.getByText("Running exactly as specified. Nothing had to be eased.")).toBeInTheDocument();
  });
});
