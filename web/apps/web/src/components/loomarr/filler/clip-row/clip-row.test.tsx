import type { ClipDTO } from "@loomarr/api";
import { LoomarrProvider } from "@loomarr/design-system";
import { fireEvent, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { ClipRow } from "./clip-row";

// LoomarrProvider as main.tsx mounts it: the select box and the Needs details chip are @loomarr/ui.
const Theme = ({ children }: { children: ReactNode }) => (
  <LoomarrProvider theme="dark">{children}</LoomarrProvider>
);
const render = (ui: ReactNode) => rtlRender(ui, { wrapper: Theme });

const base: ClipDTO = {
  name: "Fizzbolt — Dude!",
  kind: "commercial",
  durationMs: 30000,
  era: 1990,
  audience: "kids",
  category: "food",
  tagged: true,
  needsDetails: false,
  aiTagged: false,
  playCount: 0,
  playsCounted: true,
  hash: "hash-clip-test",
  tunarrProgramId: "clip-test",
};

describe("ClipRow", () => {
  it("renders the mock's columns: name, kind, era, audience and a mono duration", () => {
    render(<ClipRow clip={base} />);
    expect(screen.getByText("Fizzbolt — Dude!")).toBeInTheDocument();
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

  // Below `md` the tag columns are gone, so the phone row carries the server's verdict itself.
  it("names a commercial that needs details on the phone row, and only from the server field", () => {
    const { rerender } = render(<ClipRow clip={{ ...base, tagged: false }} />);
    expect(screen.queryByText("Needs details")).not.toBeInTheDocument();
    rerender(<ClipRow clip={{ ...base, needsDetails: true }} />);
    expect(screen.getByText("Needs details").closest(".md\\:hidden")).not.toBeNull();
  });

  // The select box is @loomarr/ui's ClipSelect, whose 24px+ target (WCAG 2.2, 2.5.8) is pinned in
  // that package's tests; here it is the row's select control and it toggles.
  it("selects the clip from its select box", () => {
    const onToggleSelect = vi.fn();
    render(<ClipRow clip={base} onToggleSelect={onToggleSelect} />);
    fireEvent.click(screen.getByRole("checkbox", { name: `Select ${base.name}` }));
    expect(onToggleSelect).toHaveBeenCalledTimes(1);
  });

  it("selects only when a handler is passed, so a member gets no dead control", () => {
    const { rerender } = render(<ClipRow clip={base} />);
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();

    const onToggleSelect = vi.fn();
    rerender(<ClipRow clip={base} onToggleSelect={onToggleSelect} />);
    fireEvent.click(screen.getByRole("checkbox", { name: /select fizzbolt/i }));
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

    // The image-service record (§22, V52 phase 8) — `thumbnail` and the route it addressed are
    // retired, so "has a frame" is now "has an adopted artwork record".
    rerender(
      <ClipRow
        clip={{
          ...base,
          thumbImage: {
            hash: "aaaa1111",
            role: "thumb",
            width: 500,
            height: 281,
            placeholder: "1QcSHQRnh493V4dIh4eXh1h4kJUI",
            dominantHex: "#2b4a5e",
            animated: false,
            srcSetWebp: "/v1/images/aaaa1111/w342.webp 342w",
            srcSetAvif: "",
            src: "/v1/images/aaaa1111/w500.jpg",
          },
        }}
      />,
    );
    expect(container.querySelector("img")).toBeInTheDocument();
  });
});
