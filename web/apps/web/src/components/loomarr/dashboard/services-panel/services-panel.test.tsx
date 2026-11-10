import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
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

  it("opens retained evidence already scoped to the failing service", async () => {
    const onDiagnose = vi.fn();
    render(
      <ServicesPanel
        view={{ loomarr, rows: [{ name: "media_server", ok: false, hint: "timed out" }] }}
        onFix={() => {}}
        onDiagnose={onDiagnose}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "View Media server diagnostics" }));
    expect(onDiagnose).toHaveBeenCalledWith("media_server");
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
    render(
      <ServicesPanel
        view={{ loomarr, rows: [{ name: "filler", ok: false, notSetUp: true }] }}
        onFix={() => {}}
      />,
    );
    expect(screen.getByText("not configured")).toBeInTheDocument();
  });

  // #3403: on a fresh install every optional service carries a hint ("set the Seerr URL") but is
  // not a failure. The server's notSetUp decides, so Health agrees with Connections.
  it("shows a fresh install's unset optional services as Not set up, not Needs attention", () => {
    render(
      <ServicesPanel
        view={{
          loomarr,
          rows: [
            { name: "media_server", ok: true, target: "http://emby.lan:8096" },
            {
              name: "requester",
              ok: false,
              hint: "set the Seerr URL",
              notSetUp: true,
              settingsGroup: "requester",
            },
            {
              name: "tunarr",
              ok: false,
              hint: "set the Tunarr URL",
              notSetUp: true,
              settingsGroup: "tunarr",
            },
            { name: "tunarr_library", ok: false, hint: "set the Tunarr URL", notSetUp: true },
            { name: "llm", ok: false, hint: "set the Ollama URL", notSetUp: true, settingsGroup: "ai" },
          ],
        }}
        onFix={() => {}}
        onDiagnose={() => {}}
      />,
    );
    expect(screen.getAllByText("Not set up")).toHaveLength(4);
    expect(screen.queryByText("Needs attention")).not.toBeInTheDocument();
    expect(screen.getByText("set the Seerr URL")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /diagnostics/i })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Fix Requester" })).toBeInTheDocument();
  });

  it("keeps a failing service red even though it carries a hint", () => {
    render(
      <ServicesPanel
        view={{ loomarr, rows: [{ name: "media_server", ok: false, hint: "connection refused" }] }}
        onFix={() => {}}
      />,
    );
    expect(screen.getByText("Needs attention")).toBeInTheDocument();
    expect(screen.queryByText("Not set up")).not.toBeInTheDocument();
  });

  // One vocabulary with Settings: the shared status words, not lower-case pass/fail.
  it("says OK, Needs attention or Not set up in words", () => {
    render(
      <ServicesPanel
        view={{
          loomarr,
          rows: [
            { name: "requester", ok: false, hint: "connection refused" },
            { name: "filler", ok: false, notSetUp: true },
          ],
        }}
        onFix={() => {}}
      />,
    );
    expect(screen.getByText("OK")).toBeInTheDocument();
    expect(screen.getByText("Needs attention")).toBeInTheDocument();
    expect(screen.getByText("Not set up")).toBeInTheDocument();
    expect(screen.queryByText("pass")).not.toBeInTheDocument();
    expect(screen.queryByText("fail")).not.toBeInTheDocument();
  });

  // #2734: TMDB has no target by design, so a passing TMDB row read "not configured · pass"
  // while Connections showed it working. A passing row is never called unconfigured.
  it("never calls a passing row without a target unconfigured", () => {
    render(<ServicesPanel view={{ loomarr, rows: [{ name: "tmdb", ok: true }] }} onFix={() => {}} />);
    expect(screen.getByText("TMDB")).toBeInTheDocument();
    expect(screen.queryByText("not configured")).not.toBeInTheDocument();
  });

  // #2734: every check the server sends reads as a name, never as its internal key.
  it.each([
    ["sonarr", "Sonarr"],
    ["radarr", "Radarr"],
    ["livetv", "Live TV guide"],
    ["tunarr_library", "Tunarr library"],
  ])("labels %s as %s", (name, label) => {
    render(<ServicesPanel view={{ loomarr, rows: [{ name, ok: true, target: "x" }] }} onFix={() => {}} />);
    expect(screen.getByText(label)).toBeInTheDocument();
    expect(screen.queryByText(name)).not.toBeInTheDocument();
  });
});
