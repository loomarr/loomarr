import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ServicesPanel } from "./services-panel";

const loomarr = { name: "loomarr", ok: true, target: "v0.9.3 · sqlite schema 21" };

describe("ServicesPanel", () => {
  it("lists Loomarr itself alongside every integration", () => {
    render(
      <ServicesPanel
        view={{ loomarr, rows: [{ name: "media_server", ok: true, target: "http://emby.lan:8096" }] }}
        onFix={() => {}}
      />,
    );

    expect(screen.getByText("Loomarr core")).toBeInTheDocument();
    expect(screen.getByText("Media server")).toBeInTheDocument();
    expect(screen.getByText("http://emby.lan:8096")).toBeInTheDocument();
  });

  // ⚠ THE V31 GATE: a failing row routes to the settings that own it. A red dot with nowhere
  // to go is a puzzle, not a diagnosis.
  it("sends a failing row to its settings", async () => {
    const onFix = vi.fn();
    render(
      <ServicesPanel
        view={{
          loomarr,
          rows: [{ name: "requester", ok: false, hint: "connection refused", settingsGroup: "requester" }],
        }}
        onFix={onFix}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Fix Requester" }));

    expect(onFix).toHaveBeenCalledWith("requester");
  });

  // The hint is the actionable half — the panel's job is to explain, not just to flag.
  it("shows the server's reason on a failing row", () => {
    render(
      <ServicesPanel
        view={{ loomarr, rows: [{ name: "requester", ok: false, hint: "connection refused" }] }}
        onFix={() => {}}
      />,
    );
    expect(screen.getByText("connection refused")).toBeInTheDocument();
  });

  // ⚠ Fix appears ONLY on a failing row. Offering it on a healthy one trains the eye past it.
  it("offers no Fix on a healthy row", () => {
    render(
      <ServicesPanel
        view={{
          loomarr,
          rows: [{ name: "tunarr", ok: true, target: "http://t:8000", settingsGroup: "tunarr" }],
        }}
        onFix={() => {}}
      />,
    );
    expect(screen.queryByRole("button", { name: /fix/i })).not.toBeInTheDocument();
  });

  // An unconfigured integration has no target; saying so beats an empty cell that reads as a
  // rendering bug.
  it("names an unconfigured integration rather than showing a blank", () => {
    render(<ServicesPanel view={{ loomarr, rows: [{ name: "filler", ok: false }] }} onFix={() => {}} />);
    expect(screen.getByText("not configured")).toBeInTheDocument();
  });
});
