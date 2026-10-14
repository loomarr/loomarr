import type { LLMModelView } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ModelPicker } from "./model-picker";

const model = (over: Partial<LLMModelView> & Pick<LLMModelView, "tag" | "label">): LLMModelView => ({
  approxVramGiB: 6,
  fit: "fits",
  pulled: true,
  recommended: false,
  runtimeOk: true,
  tools: true,
  vision: false,
  why: "why text",
  ...over,
});

describe("ModelPicker", () => {
  it("offers download for a model that isn't local yet, not selection", async () => {
    const onPull = vi.fn();
    const onSelect = vi.fn();
    render(
      <ModelPicker
        catalog={[model({ tag: "llama3.1:8b", label: "Llama", pulled: false })]}
        onSelect={onSelect}
        onPull={onPull}
      />,
    );
    // Selecting an unpulled tag 409s on the BE (§8.1), so the UI never offers it.
    expect(screen.queryByRole("button", { name: /use this/i })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /download/i }));
    expect(onPull).toHaveBeenCalledWith("llama3.1:8b");
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("shows an unrunnable model disabled with the reason, rather than hiding it", () => {
    render(
      <ModelPicker
        catalog={[model({ tag: "big:70b", label: "Big", pulled: false, fit: "wont_fit", runtimeOk: false })]}
        onSelect={vi.fn()}
        onPull={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: /download/i })).toBeDisabled();
    expect(screen.getByText(/won't fit/i)).toBeInTheDocument();
    expect(screen.getByText(/needs a newer ollama/i)).toBeInTheDocument();
  });

  it("marks the active model and does not offer to re-select it", () => {
    render(
      <ModelPicker
        catalog={[model({ tag: "qwen3:8b", label: "Qwen" })]}
        active="qwen3:8b"
        onSelect={vi.fn()}
        onPull={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: /in use/i })).toBeDisabled();
  });

  it("renders the percent the BE computed, not one derived from bytes", () => {
    render(
      <ModelPicker
        catalog={[model({ tag: "llama3.1:8b", label: "Llama", pulled: false })]}
        pulling={{ tag: "llama3.1:8b", percent: 25 }}
        onSelect={vi.fn()}
        onPull={vi.fn()}
      />,
    );
    expect(screen.getByText(/downloading 25%/i)).toBeInTheDocument();
  });
});
