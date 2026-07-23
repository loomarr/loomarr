import type { DiscoverModelView } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ModelDiscover } from "./model-discover";

const result = (
  over: Partial<DiscoverModelView> & Pick<DiscoverModelView, "id" | "quant">,
): DiscoverModelView => ({
  label: over.id,
  pullRef: `hf.co/${over.id}:${over.quant}`,
  sizeGiB: 5,
  fit: "fits",
  downloads: 1000,
  ...over,
});

describe("ModelDiscover", () => {
  it("pulls a candidate by its quant-targeted pull ref, not its bare id", async () => {
    const onPull = vi.fn();
    render(
      <ModelDiscover
        results={[result({ id: "unsloth/Qwen3.5-4B-GGUF", quant: "Q4_K_M" })]}
        onPull={onPull}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /download/i }));
    expect(onPull).toHaveBeenCalledWith("hf.co/unsloth/Qwen3.5-4B-GGUF:Q4_K_M");
  });

  it("shows the friendly label, fit verdict, quant and size for each candidate", () => {
    render(
      <ModelDiscover
        results={[
          result({ id: "u/Big-GGUF", label: "Big Model", quant: "Q3_K_S", sizeGiB: 11.5, fit: "tight" }),
        ]}
        onPull={vi.fn()}
      />,
    );
    expect(screen.getByText("Big Model")).toBeInTheDocument();
    expect(screen.getByText(/tight/i)).toBeInTheDocument();
    expect(screen.getByText(/Q3_K_S/)).toBeInTheDocument();
    expect(screen.getByText(/11\.5 GiB/)).toBeInTheDocument();
  });

  it("degrades to a huggingface.co link when the source is unreachable", () => {
    render(<ModelDiscover results={[]} sourceError onPull={vi.fn()} />);
    const link = screen.getByRole("link", { name: /browse on huggingface\.co/i });
    expect(link).toHaveAttribute("href", expect.stringContaining("huggingface.co"));
  });

  it("shows a plain empty state when nothing is compatible", () => {
    render(<ModelDiscover results={[]} onPull={vi.fn()} />);
    expect(screen.getByText(/no compatible models/i)).toBeInTheDocument();
  });
});
