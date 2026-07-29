import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AboutPanel } from "./about-panel";

const version = {
  version: "v0.9.3",
  commit: "9871fef2ac10",
  builtAt: "2026-07-22T18:04:00Z",
  ready: true,
};

describe("AboutPanel", () => {
  it("renders what the operator quotes in a bug report", () => {
    render(<AboutPanel version={version} backend="sqlite" />);

    expect(screen.getByText("v0.9.3")).toBeInTheDocument();
    expect(screen.getByText("9871fef2ac10")).toBeInTheDocument();
    expect(screen.getByText("2026-07-22T18:04:00Z")).toBeInTheDocument();
    expect(screen.getByText("sqlite")).toBeInTheDocument();
    expect(screen.getByText("Ready")).toBeInTheDocument();
  });

  // ⚠ Rows the server cannot fill are ABSENT, not blank. A source build carries no linker
  // stamps, and "Commit —" is a promise the page cannot keep.
  it("omits rows the server did not send", () => {
    render(<AboutPanel version={{ version: "dev", ready: true }} />);

    expect(screen.getByText("dev")).toBeInTheDocument();
    expect(screen.queryByText("Commit")).not.toBeInTheDocument();
    expect(screen.queryByText("Built")).not.toBeInTheDocument();
    expect(screen.queryByText("Database")).not.toBeInTheDocument();
  });

  // "v0.9.3" and "v0.9.3 plus local edits" are different things when someone is
  // diagnosing a problem, so a dirty build must not report as the clean tag.
  it("marks a build made from a modified working tree", () => {
    render(<AboutPanel version={{ ...version, dirty: true }} />);
    expect(screen.getByText("v0.9.3 (modified)")).toBeInTheDocument();
  });

  // Not-ready reports WHY: the detail is the entire reason to open this page.
  it("reports why the instance is not ready", () => {
    render(<AboutPanel version={{ version: "v0.9.3", ready: false, detail: "media server unreachable" }} />);
    expect(screen.getByText("media server unreachable")).toBeInTheDocument();
    expect(screen.queryByText("Ready")).not.toBeInTheDocument();
  });
});
