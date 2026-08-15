import type { HostedProviderView } from "@loomarr/api/models/hostedProviderView";

interface HostedModelPickerProps {
  // The curated hosted providers (OpenRouter + Custom), each with keyConfigured + live
  // models, from GET /v1/system/llm (§8.1). API keys are never included.
  providers: HostedProviderView[];
  // The active model id, if a hosted provider is currently selected.
  activeModel?: string;
  // Selecting a hosted model: provider key + model id (+ base URL for a custom provider).
  onSelect: (sel: { provider: string; model: string; baseUrl?: string }) => void;
  busy?: boolean;
  className?: string;
}

export type { HostedModelPickerProps };
