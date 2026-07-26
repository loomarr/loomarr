import type { DiscoverModelView } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ModelDiscover } from "./model-discover";

const result = (
  over: Partial<DiscoverModelView> & Pick<DiscoverModelView, "id" | "quant">,
): DiscoverModelView => ({
  label: over.id,
  pullRef: `hf.co/${over.id}`,
  sizeGiB: 5,
  fit: "fits",
  downloads: 1000,
  role: "balanced",
  recommended: false,
  note: "Best all-round pick — fits comfortably",
  ...over,
});

describe("ModelDiscover", () => {
  it("pulls a candidate by its pull ref", async () => {
    const onPull = vi.fn();
    render(
      <ModelDiscover results={[result({ id: "unsloth/Qwen3-8B-GGUF", quant: "Q4_K_M" })]} onPull={onPull} />,
    );
    await userEvent.click(screen.getByRole("button", { name: /get/i }));
    expect(onPull).toHaveBeenCalledWith("hf.co/unsloth/Qwen3-8B-GGUF");
  });

  it("tags the recommended pick with a quiet 'suggested' badge and shows its plain note", () => {
    render(
      <ModelDiscover
        results={[
          result({
            id: "u/Qwen3-8B-GGUF",
            label: "Qwen3 8B",
            quant: "Q4_K_M",
            recommended: true,
            note: "Best all-round pick — fits comfortably",
          }),
        ]}
        onPull={vi.fn()}
      />,
    );
    // A quiet inline tag — NOT a loud "recommended for your GPU" hero banner.
    expect(screen.getByText("suggested")).toBeInTheDocument();
    expect(screen.queryByText(/recommended for your/i)).not.toBeInTheDocument();
    expect(screen.getByText("Qwen3 8B")).toBeInTheDocument();
    expect(screen.getByText("Best all-round pick — fits comfortably")).toBeInTheDocument();
    // Plain size, no HF quant/downloads jargon.
    expect(screen.getByText(/5 GiB/)).toBeInTheDocument();
    expect(screen.queryByText(/Q4_K_M/)).not.toBeInTheDocument();
    expect(screen.queryByText(/downloads/)).not.toBeInTheDocument();
  });

  it("shows the role note (fit stated in plain words) without quant/download jargon", () => {
    render(
      <ModelDiscover
        results={[
          result({ id: "u/rec-GGUF", quant: "Q4_K_M", recommended: true }),
          result({
            id: "u/Big-GGUF",
            label: "Big Model",
            quant: "Q3_K_S",
            sizeGiB: 11.5,
            fit: "tight",
            role: "higher_quality",
            note: "Higher quality — fits, but tight on VRAM",
          }),
        ]}
        onPull={vi.fn()}
      />,
    );
    expect(screen.getByText("Big Model")).toBeInTheDocument();
    // Fit is conveyed by the note itself — no separate mono "tight" badge to duplicate it.
    expect(screen.getByText("Higher quality — fits, but tight on VRAM")).toBeInTheDocument();
    expect(screen.queryByText(/Q3_K_S/)).not.toBeInTheDocument();
  });

  it("degrades to a huggingface.co link when the source is unreachable", () => {
    render(<ModelDiscover results={[]} sourceError onPull={vi.fn()} />);
    const link = screen.getByRole("link", { name: /browse on huggingface\.co/i });
    expect(link).toHaveAttribute("href", expect.stringContaining("huggingface.co"));
  });

  it("shows a plain empty state when nothing is compatible", () => {
    render(<ModelDiscover results={[]} onPull={vi.fn()} />);
    expect(screen.getByText(/no other compatible models/i)).toBeInTheDocument();
  });
});
