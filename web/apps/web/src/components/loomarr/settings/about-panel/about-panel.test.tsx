import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AboutPanel } from "./about-panel";

const NOW = Date.parse("2026-07-29T12:00:00Z");
const STARTED = "2026-07-23T07:48:00Z"; // 6d 4h 12m before NOW

const version = {
  version: "v0.9.3",
  commit: "9871fef2ac10",
  builtAt: "2026-07-22T18:04:00Z",
  goVersion: "go1.23.4",
  platform: "linux/amd64",
  startedAt: STARTED,
  backend: "sqlite",
  schemaVersion: 20,
  ready: true,
};

describe("AboutPanel", () => {
  it("renders what the operator quotes in a bug report", () => {
    render(<AboutPanel version={version} now={NOW} />);

    expect(screen.getByText("v0.9.3")).toBeInTheDocument();
    expect(screen.getByText("9871fef2ac10")).toBeInTheDocument();
    expect(screen.getByText("2026-07-22T18:04:00Z")).toBeInTheDocument();
    expect(screen.getByText("go1.23.4 · linux/amd64")).toBeInTheDocument();
    expect(screen.getByText("Ready")).toBeInTheDocument();
  });

  // ⚠ Uptime is DERIVED from the server's instant, not read from a duration field. The
  // server sends startedAt precisely because a server-computed duration is stale on
  // arrival.
  it("derives uptime from the process start", () => {
    render(<AboutPanel version={version} now={NOW} />);
    expect(screen.getByText("6d 4h 12m")).toBeInTheDocument();
  });

  // Right after a restart — "just started", never "0m", which reads like a broken value.
  it("names a fresh start rather than showing zero", () => {
    render(<AboutPanel version={{ ...version, startedAt: "2026-07-29T11:59:40Z" }} now={NOW} />);
    expect(screen.getByText("just started")).toBeInTheDocument();
  });

  // The backend and the applied schema are one row: an operator diagnosing a migration
  // problem needs them together, and two separate rows makes them do the join.
  it("pairs the backend with its applied schema version", () => {
    render(<AboutPanel version={version} now={NOW} />);
    expect(screen.getByText("sqlite · schema 20")).toBeInTheDocument();
  });

  // ⚠ Rows the server cannot fill are ABSENT, not blank. A source build carries no linker
  // stamps and a store-less boot no schema; "Commit —" is a promise the page cannot keep.
  it("omits rows the server did not send", () => {
    render(<AboutPanel version={{ version: "dev", ready: true }} now={NOW} />);

    expect(screen.getByText("dev")).toBeInTheDocument();
    expect(screen.queryByText("Commit")).not.toBeInTheDocument();
    expect(screen.queryByText("Built")).not.toBeInTheDocument();
    expect(screen.queryByText("Go runtime")).not.toBeInTheDocument();
    expect(screen.queryByText("Uptime")).not.toBeInTheDocument();
    expect(screen.queryByText("Database")).not.toBeInTheDocument();
  });

  // A backend with no schema version still renders — the row must not collapse to nothing
  // just because one half is unknown.
  it("renders a backend even when the schema version is unknown", () => {
    render(<AboutPanel version={{ ...version, schemaVersion: undefined }} now={NOW} />);
    expect(screen.getByText("sqlite")).toBeInTheDocument();
    expect(screen.queryByText(/schema/)).not.toBeInTheDocument();
  });

  // "v0.9.3" and "v0.9.3 plus local edits" are different things when someone is
  // diagnosing a problem, so a dirty build must not report as the clean tag.
  it("marks a build made from a modified working tree", () => {
    render(<AboutPanel version={{ ...version, dirty: true }} now={NOW} />);
    expect(screen.getByText("v0.9.3 (modified)")).toBeInTheDocument();
  });

  // Not-ready reports WHY: the detail is the entire reason to open this page.
  it("reports why the instance is not ready", () => {
    render(
      <AboutPanel
        version={{ version: "v0.9.3", ready: false, detail: "media server unreachable" }}
        now={NOW}
      />,
    );
    expect(screen.getByText("media server unreachable")).toBeInTheDocument();
    expect(screen.queryByText("Ready")).not.toBeInTheDocument();
  });
});
