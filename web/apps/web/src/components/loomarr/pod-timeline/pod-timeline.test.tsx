import type { PodEntryDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PodTimeline } from "./pod-timeline";

const entry = (name: string, kind: PodEntryDTO["kind"], durationMs: number): PodEntryDTO => ({
  name,
  kind,
  durationMs,
  isFallbackCard: false,
  tunarrProgramId: name,
});

describe("PodTimeline", () => {
  it("lays out bumper → ads → bumper segments with era context", () => {
    render(
      <PodTimeline
        era={1990}
        audience="kids"
        entries={[
          entry("Open", "bumper", 5000),
          entry("Sunny D", "commercial", 30000),
          entry("Close", "bumper", 5000),
        ]}
      />,
    );
    expect(screen.getByRole("list", { name: /pod segments/i })).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(3);
    expect(screen.getByText("1990s")).toBeInTheDocument();
    // A legend spells out the codes actually present (bumper + commercial here), so BMP/AD
    // don't read as mystery tokens — and only those two, not every possible kind. The legend
    // text is split across <span>s (CODE = word), so match on the container's text content.
    const legend = screen.getByText((_, el) => el?.textContent === "BMP = bumper · AD = commercial");
    expect(legend).toBeInTheDocument();
    expect(legend.textContent).not.toContain("trailer");
  });

  // An exact match is the quiet case: no chip, because there is nothing to explain.
  it("shows no match chip when the pod matched exactly", () => {
    render(<PodTimeline matchLevel="exact" entries={[entry("Ad", "commercial", 30000)]} />);
    expect(screen.queryByText(/widened|ignored|bumper only/i)).not.toBeInTheDocument();
  });

  // Each rung of the §10 ladder names itself, so "why are my commercials wrong" is
  // answerable from the screen rather than from the logs.
  it.each([
    ["widened", "Era widened"],
    ["audience", "Any era"],
    ["bumper_card", "Bumper only"],
  ] as const)("names the %s fallback level", (level, label) => {
    render(<PodTimeline matchLevel={level} entries={[entry("Filler", "interstitial", 15000)]} />);
    expect(screen.getByText(label)).toBeInTheDocument();
  });

  // The embedded card carries no Tunarr program id; rendering it must not need one.
  it("renders the fallback card, which has no program id", () => {
    render(
      <PodTimeline
        matchLevel="bumper_card"
        entries={[{ name: "We'll be right back", kind: "bumper", durationMs: 5000, isFallbackCard: true }]}
      />,
    );
    expect(screen.getAllByRole("listitem")).toHaveLength(1);
  });
});
