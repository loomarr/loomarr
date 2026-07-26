import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ChannelIdent, monogramOf } from "./channel-ident";

describe("monogramOf", () => {
  it("takes the initials of the first two significant words", () => {
    expect(monogramOf("1980s Action Heroes")).toBe("AH");
    expect(monogramOf("Springfield Classics")).toBe("SC");
  });

  // A leading number reads badly as an initial: "90s Action" is "AC", not "9A".
  it("skips a leading numeric word", () => {
    expect(monogramOf("90s Action Heroes")).toBe("AH");
  });

  it("falls back to the first two letters of a single word", () => {
    expect(monogramOf("Cartoons")).toBe("CA");
  });

  // A name with nothing usable still gets a mark: a blank square reads as a failed image
  // load rather than as a channel without a logo.
  it("always produces something", () => {
    expect(monogramOf("")).toBe("CH");
    expect(monogramOf("!!! ???")).toBe("CH");
  });
});

describe("ChannelIdent", () => {
  it("renders a monogram when the channel has no logo", () => {
    // First two words, not first-and-last: "Star Trek Classics" → ST.
    render(<ChannelIdent name="Star Trek Classics" number={2} />);
    expect(screen.getByText("ST")).toBeInTheDocument();
  });

  it("renders the logo instead when one exists", () => {
    render(<ChannelIdent name="Star Trek Classics" number={2} logo="/icon.png" />);
    expect(screen.getByRole("img", { name: /Star Trek Classics logo/ })).toBeInTheDocument();
    expect(screen.queryByText("ST")).not.toBeInTheDocument();
  });

  // Colour keys on the NUMBER, so renaming a channel does not change how the rail looks.
  it("keeps the same colour when only the name changes", () => {
    const { container, rerender } = render(<ChannelIdent name="Old Name" number={3} />);
    const before = container.querySelector("span")?.getAttribute("style");
    rerender(<ChannelIdent name="Completely Different" number={3} />);
    expect(container.querySelector("span")?.getAttribute("style")).toBe(before);
  });
});
