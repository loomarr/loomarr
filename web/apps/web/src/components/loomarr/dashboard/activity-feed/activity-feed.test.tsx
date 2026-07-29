import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ActivityFeed } from "./activity-feed";

const NOW = Date.parse("2026-07-29T21:50:00Z");
const ago = (mins: number) => Math.floor((NOW - mins * 60_000) / 1000);

describe("ActivityFeed", () => {
  it("renders each entry's stored text with a relative time", () => {
    render(
      <ActivityFeed
        now={NOW}
        entries={[{ id: "a1", at: ago(3), kind: "title", level: "info", text: "Darkwing Duck landed" }]}
      />,
    );

    // ⚠ The TEXT is rendered as stored, never re-templated: a feed row is a historical
    // record, and re-rendering it against current data would let last week's entry change
    // its wording when a channel is renamed.
    expect(screen.getByText("Darkwing Duck landed")).toBeInTheDocument();
    expect(screen.getByText("3m ago")).toBeInTheDocument();
  });

  // The level drives the dot, and it is the only signal separating "a title arrived" from
  // "a channel could not reach Tunarr" at a glance.
  it("marks each entry with its level", () => {
    render(
      <ActivityFeed
        now={NOW}
        entries={[
          { id: "a1", at: ago(1), kind: "system", level: "warn", text: "Seerr timed out" },
          { id: "a2", at: ago(2), kind: "channel", level: "error", text: "CH 12 unreachable" },
        ]}
      />,
    );
    expect(screen.getByRole("img", { name: "warn" })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "error" })).toBeInTheDocument();
  });

  // A fresh install has done nothing yet. An explanation beats an empty box, which reads as
  // a panel that failed to load.
  it("explains an empty feed", () => {
    render(<ActivityFeed now={NOW} entries={[]} />);
    expect(screen.getByText(/nothing yet/i)).toBeInTheDocument();
  });
});
