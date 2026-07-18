import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Clip } from "../clip-card";
import { PodTimeline } from "./pod-timeline";

const clip = (name: string, kind: Clip["kind"], durationMs: number): Clip => ({
  name,
  kind,
  durationMs,
  tagged: true,
  aiTagged: false,
});

describe("PodTimeline", () => {
  it("lays out bumper → ads → bumper segments with era context", () => {
    render(
      <PodTimeline
        era={1990}
        audience="kids"
        clips={[
          clip("Open", "bumper", 5000),
          clip("Sunny D", "commercial", 30000),
          clip("Close", "bumper", 5000),
        ]}
      />,
    );
    expect(screen.getByRole("list", { name: /pod segments/i })).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(3);
    expect(screen.getByText("1990s")).toBeInTheDocument();
  });

  it("flags a widened fallback match", () => {
    render(<PodTimeline match="fallback-widened" clips={[clip("Filler", "interstitial", 15000)]} />);
    expect(screen.getByText("Widened match")).toBeInTheDocument();
  });
});
