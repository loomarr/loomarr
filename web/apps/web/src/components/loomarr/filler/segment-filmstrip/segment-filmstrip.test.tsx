import type { ImageDTO } from "@loomarr/api/models/imageDTO";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SegmentFilmstrip } from "./segment-filmstrip";
import type { FilmstripSegment } from "./segment-filmstrip.type";

const seg = (key: string, startMs: number, endMs: number, over: Partial<FilmstripSegment> = {}) =>
  ({ key, startMs, endMs, ...over }) as FilmstripSegment;

const artwork: ImageDTO = {
  animated: false,
  dominantHex: "#27384a",
  hash: "frame-a",
  height: 180,
  placeholder: "",
  role: "thumb",
  src: "/frame-a.jpg",
  srcSetAvif: "",
  srcSetWebp: "/frame-a.webp 320w",
  width: 320,
};

const renderStrip = (node: React.ReactNode) => render(<TooltipProvider delay={0}>{node}</TooltipProvider>);

describe("SegmentFilmstrip", () => {
  it("renders real stills in time order and reports a chosen clip", async () => {
    const onSelect = vi.fn();
    renderStrip(
      <SegmentFilmstrip
        segments={[
          seg("second", 30_000, 60_000, { name: "Second" }),
          seg("first", 0, 30_000, { name: "First", artwork }),
        ]}
        onSelect={onSelect}
      />,
    );

    const list = screen.getByRole("list", { name: /detected clips/i });
    const buttons = within(list).getAllByRole("button");
    expect(buttons.map((button) => button.getAttribute("aria-label"))).toEqual([
      "00:00 · First",
      "00:30 · Second",
    ]);
    expect((buttons[0] as HTMLElement).querySelector("img")).toHaveAttribute("src", artwork.src);
    expect(within(buttons[1] as HTMLElement).getByText("Preview unavailable")).toBeInTheDocument();

    await userEvent.click(buttons[1] as HTMLElement);
    expect(onSelect).toHaveBeenCalledWith("second", buttons[1]);
  });

  it("keeps duration proportions while giving every clip a usable minimum width", () => {
    renderStrip(
      <SegmentFilmstrip
        segments={[seg("short", 0, 10_000, { name: "Short" }), seg("long", 10_000, 40_000, { name: "Long" })]}
      />,
    );

    const list = screen.getByRole("list", { name: /detected clips/i });
    expect(list).toHaveStyle({
      gridTemplateColumns: "minmax(6rem, 10000fr) minmax(6rem, 30000fr)",
      minWidth: "480px",
    });
  });

  it("scrolls a 50-clip reel instead of squeezing it into tiny targets", () => {
    renderStrip(
      <SegmentFilmstrip
        segments={Array.from({ length: 50 }, (_, index) =>
          seg(`clip-${index}`, index * 30_000, (index + 1) * 30_000, { name: `Clip ${index + 1}` }),
        )}
      />,
    );

    const list = screen.getByRole("list", { name: /detected clips/i });
    expect(within(list).getAllByRole("button")).toHaveLength(50);
    expect(list).toHaveStyle({ minWidth: "4800px" });
  });

  it("shows unassigned time rather than hiding a gap", () => {
    renderStrip(
      <SegmentFilmstrip
        segments={[
          seg("first", 0, 10_000, { name: "First" }),
          seg("second", 15_000, 25_000, { name: "Second" }),
        ]}
      />,
    );

    expect(screen.getByLabelText("00:10–00:15 unassigned")).toBeInTheDocument();
  });

  it("shows the same useful details on hover and keyboard focus", async () => {
    renderStrip(
      <SegmentFilmstrip
        segments={[
          seg("toy", 65_000, 95_000, {
            name: "Toy ad",
            tags: ["commercial", "animation"],
            language: "en",
            attention: "Loomarr may have missed a cut here.",
          }),
        ]}
      />,
    );

    const clip = screen.getByRole("button", { name: "01:05 · Toy ad" });
    await userEvent.hover(clip);
    const descriptionId = clip.getAttribute("aria-describedby");
    expect(descriptionId).toBeTruthy();
    expect(screen.getByRole("tooltip")).toHaveAttribute("id", descriptionId);
    expect(await screen.findByText(/commercial · animation · english/i)).toBeInTheDocument();
    expect(screen.getByText(/may have missed a cut/i)).toBeInTheDocument();
    expect(screen.getByText(/click to play this exact clip/i)).toBeInTheDocument();

    await userEvent.unhover(clip);
    clip.focus();
    expect(await screen.findByText(/commercial · animation · english/i)).toBeInTheDocument();
  });

  it("marks the active clip and handles empty duration honestly", () => {
    const { rerender } = renderStrip(
      <SegmentFilmstrip
        segments={[seg("one", 0, 5000, { name: "One" }), seg("two", 5000, 9000, { name: "Two" })]}
        activeKey="two"
      />,
    );
    expect(screen.getByRole("button", { name: /Two/ })).toHaveAttribute("aria-current", "true");
    expect(screen.getByRole("button", { name: /One/ })).not.toHaveAttribute("aria-current");

    rerender(
      <TooltipProvider delay={0}>
        <SegmentFilmstrip segments={[seg("zero", 1000, 1000)]} />
      </TooltipProvider>,
    );
    expect(screen.queryByRole("list", { name: /detected clips/i })).toBeNull();
  });
});
