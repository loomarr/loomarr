import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { NowNextStrip } from "./now-next-strip";

describe("NowNextStrip", () => {
  it("shows now and next titles", () => {
    render(<NowNextStrip now={{ title: "Heat", until: "9:42 PM" }} next={{ title: "Con Air" }} />);
    expect(screen.getByText("Heat")).toBeInTheDocument();
    expect(screen.getByText("Con Air")).toBeInTheDocument();
  });

  it("labels a flex/pod gap as Filler, not dead air", () => {
    render(<NowNextStrip gap />);
    expect(screen.getByText("Filler")).toBeInTheDocument();
  });

  it("reads 'Nothing scheduled' when empty", () => {
    render(<NowNextStrip />);
    expect(screen.getByText("Nothing scheduled")).toBeInTheDocument();
  });
});
