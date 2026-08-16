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
  it("offers explicit behavioral verification for an unverified hosted model", async () => {
    const onSelect = vi.fn();
    const onVerify = vi.fn();
    render(
      <HostedModelPicker
        providers={[
          provider({
            models: [
              {
                id: "vendor/thin-model",
                label: "Thin metadata model",
                toolCapability: "unverified",
              },
            ],
          }),
        ]}
        onSelect={onSelect}
        onVerify={onVerify}
      />,
    );

    expect(screen.queryByRole("button", { name: /use this/i })).not.toBeInTheDocument();
    expect(screen.getByText(/one small inference call/i)).toBeInTheDocument();
    expect(screen.getByText(/does not certify curation quality/i)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /verify tool calling/i }));
    expect(onVerify).toHaveBeenCalledWith({
      provider: "openrouter",
      model: "vendor/thin-model",
      baseUrl: "https://openrouter.ai/api/v1",
    });
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("selects a hosted model with its provider key + base URL", async () => {
    const onSelect = vi.fn();
    render(
      <HostedModelPicker
        providers={[
          provider({
            models: [
              { id: "openai/gpt-4o-mini", label: "GPT-4o mini", tools: true, toolCapability: "verified" },
            ],
          }),
        ]}
        onSelect={onSelect}
        onVerify={vi.fn()}
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
        providers={[
          provider({
            active: true,
            models: [{ id: "m1", label: "Model 1", toolCapability: "verified" }],
          }),
        ]}
        activeModel="m1"
        onSelect={vi.fn()}
        onVerify={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: /in use/i })).toBeDisabled();
  });

  it("points at where to get a key when none is configured", () => {
    render(
      <HostedModelPicker
        providers={[provider({ keyConfigured: false, modelsLive: false, models: [] })]}
        onSelect={vi.fn()}
        onVerify={vi.fn()}
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
                toolCapability: "verified",
                why: "Cheap, tool-capable, and a good default for Loomarr's grounded suggestions.",
              },
            ],
          }),
        ]}
        onSelect={vi.fn()}
        onVerify={vi.fn()}
      />,
    );

    expect(screen.getByText("GPT-4o mini")).toBeInTheDocument();
    expect(screen.getByText("recommended")).toBeInTheDocument();
    expect(screen.getByText("Tools verified")).toBeInTheDocument();
    expect(screen.getByText(/good default for Loomarr/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /add key first/i })).toBeDisabled();
  });

  it("marks a model without advertised tool calling as unusable", () => {
    render(
      <HostedModelPicker
        providers={[
          provider({
            models: [
              { id: "vendor/text-only", label: "Text only", tools: false, toolCapability: "unsupported" },
            ],
          }),
        ]}
        onSelect={vi.fn()}
        onVerify={vi.fn()}
      />,
    );

    expect(screen.getByText("Tools unsupported")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /can't use/i })).toBeDisabled();
  });
});
