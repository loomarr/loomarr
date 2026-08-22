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
      <HostedModelPicker
        providers={[provider({ keyConfigured: false, modelsLive: false, models: [] })]}
        onSelect={vi.fn()}
      />,
    );
    expect(screen.getByText(/save to use the suggested model/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /get a key/i })).toHaveAttribute(
      "href",
      "https://openrouter.ai/keys",
    );
  });

  it("shows the guided fallback before a key is configured", () => {
    render(
      <HostedModelPicker
        providers={[
          provider({
            keyConfigured: false,
            modelsLive: false,
            models: [
              {
                id: "openai/gpt-4o-mini",
                label: "GPT-4o mini",
                recommended: true,
                tools: true,
                why: "Cheap, tool-capable, and a good default for Loomarr's grounded suggestions.",
              },
            ],
          }),
        ]}
        onSelect={vi.fn()}
      />,
    );

    expect(screen.getByText("GPT-4o mini")).toBeInTheDocument();
    expect(screen.getByText("recommended")).toBeInTheDocument();
    expect(screen.getByText("Tools")).toBeInTheDocument();
    expect(screen.getByText(/good default for Loomarr/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /add key first/i })).toBeDisabled();
  });

  it("marks a model without advertised tool calling as unusable", () => {
    render(
      <HostedModelPicker
        providers={[provider({ models: [{ id: "vendor/text-only", label: "Text only", tools: false }] })]}
        onSelect={vi.fn()}
      />,
    );

    expect(screen.getByText("Tools required")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /can't use/i })).toBeDisabled();
  });

  it("leads with one default, limits alternatives, and progressively discloses the catalog", async () => {
    const models = [
      {
        id: "primary",
        label: "Primary",
        recommended: true,
        tools: true,
        why: "GPT-4.1 — strong grounded tool-caller, ~$1.00/1M tokens",
      },
      {
        id: "duplicate",
        label: "Same family variant",
        tools: true,
        why: "GPT-4.1 — strong grounded tool-caller, ~$2.00/1M tokens",
      },
      { id: "fast", label: "Fast", tools: true, why: "Claude Haiku — lower cost" },
      { id: "quality", label: "Quality", tools: true, why: "Gemini Pro — more context" },
      { id: "advanced", label: "Advanced", tools: true },
    ];
    render(<HostedModelPicker providers={[provider({ models })]} onSelect={vi.fn()} />);

    expect(screen.getByText("Safe default")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /use recommended/i })).toBeInTheDocument();
    expect(screen.getByText("Strong alternatives")).toBeInTheDocument();
    expect(screen.getByText("Fast")).toBeInTheDocument();
    expect(screen.getByText("Quality")).toBeInTheDocument();
    expect(screen.queryByText("Same family variant")).not.toBeInTheDocument();
    expect(screen.queryByText("Advanced")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /show all 5 models/i }));
    expect(screen.getByText("Same family variant")).toBeInTheDocument();
    expect(screen.getByText("Advanced")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /show guided choices/i })).toBeInTheDocument();
  });

  it("keeps an active model outside the guided shortlist visible", () => {
    render(
      <HostedModelPicker
        providers={[
          provider({
            active: true,
            models: [
              { id: "primary", label: "Primary", recommended: true, tools: true },
              { id: "fast", label: "Fast", tools: true },
              { id: "quality", label: "Quality", tools: true },
              { id: "active", label: "Current model", tools: true },
            ],
          }),
        ]}
        activeModel="active"
        onSelect={vi.fn()}
      />,
    );

    expect(screen.getByText("Current model")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /in use/i })).toBeDisabled();
  });
});
