import type { FetchFillerSourceOutputBody } from "@loomarr/api/models/fetchFillerSourceOutputBody";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { fillerSource } from "@/test/fixtures/filler";
import { checkResultText, readinessLabel, SourceStatus } from "./source-status";

const result = (over: Partial<FetchFillerSourceOutputBody> = {}): FetchFillerSourceOutputBody => ({
  sourceId: "folder",
  sourcesPolled: 1,
  queued: 0,
  skipped: 0,
  maxPerCheck: 10,
  total: 0,
  added: 0,
  updated: 0,
  pruned: 0,
  ...over,
});

describe("checkResultText", () => {
  const source = fillerSource({ target: "/data/filler" });

  it("says the limit is full before anything else", () => {
    expect(checkResultText(source, result({ stoppedBy: "catalog", queued: 3 }))).toMatch(
      /clip limit is full\. This check could add up to 10 clips\./,
    );
  });

  it("counts queued clips and mentions the ones already known", () => {
    expect(checkResultText(source, result({ queued: 1, skipped: 2, maxPerCheck: 1 }))).toBe(
      "1 new clip was queued from /data/filler. This check could add up to 1 clip. 2 clips were already known and skipped.",
    );
  });

  it("reports added and updated clips when nothing was queued", () => {
    expect(checkResultText(source, result({ added: 1, updated: 0 }))).toBe(
      "Found 1 new and 0 updated clip in /data/filler.",
    );
  });

  it("says plainly when nothing was found", () => {
    expect(checkResultText(source, result())).toBe(
      "No new clips were found in /data/filler. This check could add up to 10 clips.",
    );
  });
});

describe("readinessLabel", () => {
  it("names each state an operator can act on", () => {
    expect(readinessLabel(fillerSource({ readiness: "off" }))).toBe("Paused");
    expect(readinessLabel(fillerSource({ readiness: "provider_off" }))).toBe("Paused");
    expect(readinessLabel(fillerSource({ readiness: "needs_location" }))).toBe("Location needed");
    expect(readinessLabel(fillerSource({ readiness: "not_configured" }))).toBe("Setup needed");
    expect(readinessLabel(fillerSource({ readiness: "ready" }))).toBe("Ready");
  });
});

describe("SourceStatus", () => {
  it("summarises the source and links held clips to Incoming", () => {
    render(
      <SourceStatus
        source={fillerSource({ count: 4, incoming: 2, kind: "folder", uri: "/data/ads" })}
        fetching={false}
        onFetch={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "Ready" })).toBeVisible();
    expect(screen.getByText(/4 ready ·/)).toBeVisible();
    expect(screen.getByRole("link", { name: "2 being checked" })).toHaveAttribute("href", "/filler/incoming");
    expect(screen.getByText("/data/ads")).toBeVisible();
    expect(screen.getByText(/not checked yet/)).toBeVisible();
  });

  it("runs a check and shows what it found, only for the source it ran on", async () => {
    const onFetch = vi.fn((id: string, onResult: (r: FetchFillerSourceOutputBody) => void) =>
      onResult(result({ sourceId: id, added: 2 })),
    );
    const source = fillerSource();
    const { rerender } = render(<SourceStatus source={source} fetching={false} onFetch={onFetch} />);

    await userEvent.click(screen.getByRole("button", { name: "Look for new clips" }));

    expect(onFetch).toHaveBeenCalledWith("folder", expect.any(Function));
    expect(screen.getByRole("status")).toHaveTextContent("Found 2 new and 0 updated clips in /data/filler.");

    rerender(
      <SourceStatus
        source={fillerSource({ id: "library", kind: "library" })}
        fetching={false}
        onFetch={onFetch}
      />,
    );
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("shows progress and a failure from the fetch", () => {
    render(
      <SourceStatus
        source={fillerSource()}
        fetching
        fetchError={{ detail: "Could not reach it." }}
        onFetch={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Looking in /data/filler…" })).toBeDisabled();
    expect(screen.getByRole("alert")).toHaveTextContent("Could not reach it.");
  });

  it("hides the check button for a source that cannot be fetched", () => {
    render(
      <SourceStatus source={fillerSource({ actions: ["enable"] })} fetching={false} onFetch={vi.fn()} />,
    );
    expect(screen.queryByRole("button", { name: "Look for new clips" })).not.toBeInTheDocument();
  });
});
