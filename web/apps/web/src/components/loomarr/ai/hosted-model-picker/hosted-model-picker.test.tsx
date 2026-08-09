import type { HostedProviderView } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { HostedModelPicker } from "./hosted-model-picker";

const provider = (over: Partial<HostedProviderView>): HostedProviderView => ({
  key: "openrouter",
  label: "OpenRouter",
  baseUrl: "https://openrouter.ai/api/v1",
  keysUrl: "https://openrouter.ai/keys",
  keyConfigured: true,
  active: false,
  modelsLive: true,
  models: [],
  ...over,
});

describe("HostedModelPicker", () => {
  it("selects a hosted model with its provider key + base URL", async () => {
    const onSelect = vi.fn();
    render(
      <HostedModelPicker
        providers={[provider({ models: [{ id: "openai/gpt-4o-mini", label: "GPT-4o mini", tools: true }] })]}
        onSelect={onSelect}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /use this/i }));
    expect(onSelect).toHaveBeenCalledWith({
      provider: "openrouter",
      model: "openai/gpt-4o-mini",
      baseUrl: "https://openrouter.ai/api/v1",
    });
  });

  it("marks the active model in use and doesn't re-offer it", () => {
    render(
      <HostedModelPicker
        providers={[provider({ active: true, models: [{ id: "m1", label: "Model 1" }] })]}
        activeModel="m1"
        onSelect={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: /in use/i })).toBeDisabled();
  });

  it("points at where to get a key when none is configured", () => {
    render(
      <HostedModelPicker providers={[provider({ keyConfigured: false, models: [] })]} onSelect={vi.fn()} />,
    );
    expect(screen.getByText(/press Test to list its models/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /get a key/i })).toHaveAttribute(
      "href",
      "https://openrouter.ai/keys",
    );
  });
});
