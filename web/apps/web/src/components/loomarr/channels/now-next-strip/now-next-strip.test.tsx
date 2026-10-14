import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { NowNextStrip } from "./now-next-strip";

describe("NowNextStrip", () => {
  it("shows now and next titles", () => {
    render(
      <NowNextStrip
        now={{ title: "The Velvet Cartographer", until: "9:42 PM" }}
        next={{ title: "Pelican Point Motel" }}
      />,
    );
    expect(screen.getByText("The Velvet Cartographer")).toBeInTheDocument();
    expect(screen.getByText("Pelican Point Motel")).toBeInTheDocument();
  });

  it("labels a flex/pod gap as a filler break, not dead air", () => {
    render(<NowNextStrip gap />);
    expect(screen.getByText("Filler break")).toBeInTheDocument();
  });

  it("reads 'Nothing scheduled' when empty", () => {
    render(<NowNextStrip />);
    expect(screen.getByText("Nothing scheduled")).toBeInTheDocument();
  });
});
