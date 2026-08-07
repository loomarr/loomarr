import type { ClipDTO } from "@loomarr/api";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ClipRow } from "./clip-row";

const base: ClipDTO = {
  name: "Sunny D — Dude!",
  kind: "commercial",
  durationMs: 30000,
  era: 1990,
  audience: "kids",
  category: "food",
  tagged: true,
  aiTagged: false,
  playCount: 0,
  playsCounted: true,
  hash: "hash-clip-test",
  tunarrProgramId: "clip-test",
};

describe("ClipRow", () => {
  it("renders the mock's columns: name, kind, era, audience and a mono duration", () => {
    render(<ClipRow clip={base} />);
    expect(screen.getByText("Sunny D — Dude!")).toBeInTheDocument();
    expect(screen.getByText("Commercial")).toBeInTheDocument();
    expect(screen.getByText("1990s")).toBeInTheDocument();
    expect(screen.getByText("Kids")).toBeInTheDocument();
    expect(screen.getByText("30s")).toBeInTheDocument();
  });

  it("renders an em-dash for an unset era or audience, not a blank cell", () => {
    render(<ClipRow clip={{ ...base, era: undefined, audience: undefined }} />);
    // Two columns, both unset — a blank would be indistinguishable from a column that
    // failed to render.
    expect(screen.getAllByText("—")).toHaveLength(2);
  });

  it("selects only when a handler is passed, so a member gets no dead control", () => {
    const { rerender } = render(<ClipRow clip={base} />);
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();

    const onToggleSelect = vi.fn();
    rerender(<ClipRow clip={base} onToggleSelect={onToggleSelect} />);
    fireEvent.click(screen.getByRole("checkbox", { name: /select sunny d/i }));
    expect(onToggleSelect).toHaveBeenCalledOnce();
  });

  // ⚠ The three-branch airings rule, shared with ClipCard via `playsLine` (§10 V45a: "aired", not
  // "played" — the counter is broadcasts). `playsCounted:false` is NOT zero airings — it means this
  // install cannot OBSERVE airings — so "Never aired" here would be a falsehood, not a rounding.
  it("says airings aren't counted rather than claiming zero, when the install can't observe them", () => {
    render(<ClipRow clip={{ ...base, playsCounted: false, playCount: 0 }} />);
    expect(screen.getByText(/airings aren't counted on this setup/i)).toBeInTheDocument();
    expect(screen.queryByText(/never aired/i)).not.toBeInTheDocument();
  });

  it("distinguishes a genuinely never-aired clip from an uncounted one", () => {
    render(<ClipRow clip={{ ...base, playsCounted: true, playCount: 0 }} />);
    expect(screen.getByText(/never aired/i)).toBeInTheDocument();
  });

  // The thumbnail column holds its box either way — in a list, omitting the element would
  // ragged the name column against neighbouring rows.
  it("keeps the thumbnail column when a clip has no extracted frame", () => {
    const { container, rerender } = render(<ClipRow clip={base} />);
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector(".w-\\[54px\\]")).toBeInTheDocument();

    rerender(<ClipRow clip={{ ...base, thumbnail: "clip-test.jpg" }} />);
    expect(container.querySelector("img")).toBeInTheDocument();
  });
});
