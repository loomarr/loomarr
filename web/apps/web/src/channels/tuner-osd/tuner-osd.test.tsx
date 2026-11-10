import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TunerOSD } from "./tuner-osd";

describe("TunerOSD", () => {
  it("announces the requested channel and current programme while tuning", () => {
    render(<TunerOSD number={42} name="Late Night Noir" currentTitle="The Big Sleep" />);
    const status = screen.getByRole("status");
    expect(status).toHaveTextContent("CH 42");
    expect(status).toHaveTextContent("Late Night Noir");
    expect(status).toHaveTextContent("The Big Sleep");
    expect(status).toHaveTextContent("Tuning…");
  });

  // The wash's channel line is what a viewer sees; this is the announcement only (#1620, #2769).
  it("is for screen readers only", () => {
    render(<TunerOSD number={42} name="Late Night Noir" />);
    expect(screen.getByRole("status")).toHaveClass("sr-only");
  });
});
