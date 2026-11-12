import * as setupApi from "@loomarr/api/endpoints/setup";
import * as systemApi from "@loomarr/api/endpoints/system";
import type { HostedProviderView } from "@loomarr/api/models/hostedProviderView";
import type { SettingKey } from "@loomarr/api/models/settingKey";
import type { SystemLLMStatus } from "@loomarr/api/models/systemLLMStatus";
import { unwrap } from "@loomarr/api/unwrap";
import { Action, Disclosure, RadioGroup, RadioItem, Stack, Surface, Text } from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { HostedModelPicker } from "@/components/loomarr/ai/hosted-model-picker";
import { ModelDiscover } from "@/components/loomarr/ai/model-discover";
import { ModelPicker } from "@/components/loomarr/ai/model-picker";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useModelPull } from "@/settings/use-model-pull";
import {
  REUSE_LINEUP,
  sameVisionChoice,
  type VisionChoice,
  visionChoiceOf,
  visionChoiceSettings,
} from "@/settings/vision-choice";

// A hosted provider's lineup: pick a tool-capable model from its live /models list.
//
// ⚠ No test button of its own (#2735). The connection block's check is the one test: it fails
// when this list cannot be read, so the card never says "Connection OK" above "No models
// returned". The key itself is validated when a model is selected.
const HostedLineup = ({
  status,
  provider,
  onModelChange,
}: {
  status: SystemLLMStatus;
  provider?: HostedProviderView;
  onModelChange: () => void;
}) => {
  const select = systemApi.useSystemLlmSelect({ mutation: { onSuccess: onModelChange } });
  const providers = provider
    ? [{ ...provider, models: provider.models?.filter((model) => model.tools) }]
    : [];

  return (
    <Stack gap="control">
      {select.error != null && <ErrorState error={select.error} />}
      <HostedModelPicker
        providers={providers}
        activeModel={status.model}
        busy={select.isPending}
        onSelect={(sel) => select.mutate({ data: sel })}
      />
    </Stack>
  );
};

// A local Ollama's lineup gets two lists: the INSTALLED models (fit-ranked, tool-capable, select
// or grey-out) and the ones compatible to DOWNLOAD (popular Hugging Face GGUF models sized
// against this machine's VRAM, best-first). Ollama ships no "what can I download" API, so the
// catalog is what you've pulled plus a machine-ranked browse, never a hardcoded list.
const OllamaLineup = ({ status, onModelChange }: { status: SystemLLMStatus; onModelChange: () => void }) => {
  const select = systemApi.useSystemLlmSelect({ mutation: { onSuccess: onModelChange } });
  const pull = useModelPull();
  const discover = systemApi.useSystemLlmDiscover({ query: { retry: false } });
  const discoverBody = unwrap(discover.data);
  const discoverFailed =
    (discover.data !== undefined && discover.data.status !== 200) || discoverBody?.sourceOk === false;
  const installed = (status.catalog ?? []).filter((model) => model.tools);

  return (
    <Stack gap="5">
      {select.error != null && <ErrorState error={select.error} />}
      {discover.error != null && <ErrorState error={discover.error} onRetry={() => discover.refetch()} />}
      {pull.error != null && <ErrorState error={pull.error} />}
      {installed.length > 0 ? (
        <ModelPicker
          catalog={installed}
          active={status.model}
          gpuName={status.gpuName}
          vramGiB={status.vramGiB}
          busy={select.isPending}
          pulling={pull.pulling}
          onSelect={(model) => select.mutate({ data: { model } })}
          onPull={pull.start}
        />
      ) : (
        <Text as="p" textRole="compact" tone="secondary">
          No tool-capable models installed yet. Download one below to get started.
        </Text>
      )}
      <ModelDiscover
        results={discoverBody?.models ?? undefined}
        loading={discover.isFetching}
        sourceError={discoverFailed}
        pulling={pull.pulling}
        onPull={pull.start}
      />
    </Stack>
  );
};

const TmdbNotice = ({ configured }: { configured: boolean }) => (
  <Stack
    align="center"
    className="border-border bg-muted/40 rounded-md border p-3"
    direction="row"
    gap="control"
    justify="between"
    render="section"
    wrap
  >
    <div>
      <Text as="p" textRole="cardLabel">
        {configured ? "TMDB key configured" : "TMDB grounding is still needed"}
      </Text>
      <Text as="p" textRole="compact" tone="secondary">
        {configured
          ? "Loomarr can use TMDB to ground channel suggestions in real titles."
          : "Your AI connection is separate. Connect TMDB before building a channel from a description."}
      </Text>
    </div>
    {!configured && (
      <Action
        accessibilityRole="link"
        render={<Link to="/settings/connections" search={{ focus: "tmdb" }} />}
        variant="outline"
      >
        Connect TMDB
      </Action>
    )}
  </Stack>
);

type VisionOption = { choice: VisionChoice; label: string; detail: string };

const visionKey = (choice: VisionChoice) => `${choice.provider}:${choice.model}`;

// The first three choices are guided; the rest wait behind "Show all", except the active one,
// which always shows.
const VisionPicker = ({
  options,
  active,
  onSelect,
}: {
  options: VisionOption[];
  active: VisionChoice;
  onSelect: (choice: VisionChoice) => void;
}) => {
  const [expanded, setExpanded] = useState(false);
  const guided = options.slice(0, 3);
  const current = options.find(
    (option) =>
      sameVisionChoice(option.choice, active) &&
      !guided.some((item) => sameVisionChoice(item.choice, active)),
  );
  const shown = expanded ? options : current ? [...guided, current] : guided;

  return (
    <Stack
      aria-labelledby="role-vision"
      className="border-border rounded-md border p-3"
      gap="inline"
      render="section"
    >
      <div>
        <Text as="h3" id="role-vision" textRole="cardLabel">
          Vision
        </Text>
        <Text as="p" textRole="compact" tone="secondary">
          Reads sampled filler frames. It inherits the lineup model unless you choose a vision-capable model.
        </Text>
      </div>
      {/* One choice of several, so a radio group: the current model reads as checked rather than
          as a disabled button, and arrow keys move between models. */}
      <RadioGroup
        borderWidth={0}
        gap={8}
        label="Vision model"
        onValueChange={(value) => {
          const option = shown.find((item) => visionKey(item.choice) === value);
          if (option) onSelect(option.choice);
        }}
        value={shown.some((item) => sameVisionChoice(item.choice, active)) ? visionKey(active) : undefined}
        values={shown.map((item) => visionKey(item.choice))}
      >
        {shown.map((option) => (
          <RadioItem key={visionKey(option.choice)} label={option.label} value={visionKey(option.choice)}>
            {({ checked }) => (
              <Surface
                backgroundColor={checked ? "surfaceFocus" : "surfaceRaised"}
                borderColor={checked ? "contentPrimary" : "borderControl"}
                borderRadius="control"
                borderWidth={checked ? 2 : 1}
                gap={2}
                paddingHorizontal={12}
                paddingVertical={8}
              >
                <Text strong={checked} textRole="cardLabel">
                  {option.label}
                </Text>
                <Text textRole="hint" tone="secondary">
                  {option.detail}
                </Text>
              </Surface>
            )}
          </RadioItem>
        ))}
      </RadioGroup>
      {options.length > 3 && (
        <Action onPress={() => setExpanded(!expanded)} style={{ alignSelf: "flex-start" }} variant="ghost">
          {expanded ? "Show guided choices" : `Show all ${options.length} choices`}
        </Action>
      )}
    </Stack>
  );
};

// The vision role: reuse the lineup model, or name a vision-capable model from the local
// catalog or, with a hosted lineup, from that provider.
const VisionSection = ({
  status,
  hosted,
  provider,
  active,
  onChange,
}: {
  status: SystemLLMStatus;
  hosted: boolean;
  provider?: HostedProviderView;
  active: VisionChoice;
  onChange: (choice: VisionChoice) => void;
}) => {
  const catalog = status.catalog ?? [];
  const lineup = hosted
    ? provider?.models?.find((model) => model.id === status.model)
    : catalog.find((model) => model.tag === status.model);
  const options: VisionOption[] = [
    {
      choice: REUSE_LINEUP,
      label: "Reuse the text-role choice",
      detail: lineup?.vision
        ? "This model advertises image input, so no separate vision model is needed."
        : "Inherits the text role. Choose a vision model below when that model cannot see images.",
    },
    ...catalog
      .filter((model) => model.vision)
      .map((model) => ({
        choice: { provider: "ollama" as const, model: model.tag },
        label: model.label,
        detail: `Local Ollama · ${model.fit.replace("_", " ")} · ${model.why}`,
      })),
    ...(hosted && provider?.keyConfigured
      ? (provider.models ?? [])
          .filter((model) => model.vision)
          .map((model) => ({
            choice: { provider: "inherit" as const, model: model.id },
            label: model.label,
            detail: `${provider.label} · vision capable`,
          }))
      : []),
  ];

  return (
    <Disclosure
      description="Choose a separate model only when the lineup model cannot read clip frames."
      label="Advanced vision model"
    >
      <VisionPicker options={options} active={active} onSelect={onChange} />
    </Disclosure>
  );
};

// The model picker (config-design §8.1), wired. Selecting hot-swaps the running suggester (no
// restart), which is why this sits beside the AI settings form rather than inside it: the form
// writes llm.* through PATCH, while select and pull are actions with their own lifecycles.
//
// `provider` is the LIVE (possibly unsaved) provider from the settings form, so the right lineup
// appears the instant the dropdown flips. It doesn't wait for Save, matching how the URL and key
// fields already react live.
//
// onModelChange fires after a successful model select: the seam a host (the wizard) uses to
// refresh what this component doesn't own, like the setup checklist whose `llm` verdict flips
// green once a model is active. Settings passes nothing; its own status invalidation is enough.
//
// onRoleSettingChange stages the vision role into the host's settings edit buffer. Without it
// only the lineup renders (the wizard).
const AiModelSettings = ({
  provider,
  baseUrl,
  onModelChange,
  visionProvider = "inherit",
  visionModel = "",
  onRoleSettingChange,
  tmdbConfigured,
}: {
  provider?: string;
  baseUrl?: string;
  onModelChange?: () => void;
  visionProvider?: string;
  visionModel?: string;
  onRoleSettingChange?: (key: SettingKey, value: string) => void;
  tmdbConfigured?: boolean;
}) => {
  const queryClient = useQueryClient();
  // Status carries BOTH the local catalog and the hosted-provider catalog, so it's always
  // fetched; the lineup below picks which to render from the same payload.
  const llm = systemApi.useSystemLlmStatus({ query: { retry: false } });
  const status = unwrap(llm.data);
  const modelChanged = () => {
    void queryClient.invalidateQueries({ queryKey: systemApi.getSystemLlmStatusQueryKey() });
    void queryClient.invalidateQueries({ queryKey: setupApi.getSetupStatusQueryKey() });
    onModelChange?.();
  };

  if (llm.error) return <ErrorState error={llm.error} onRetry={() => llm.refetch()} />;
  if (!status)
    return (
      <Text as="p" textRole="compact" tone="secondary">
        Checking your AI provider…
      </Text>
    );

  const hosted = provider === "openai";
  const candidates = status.hosted ?? [];
  const activeProvider =
    candidates.find((candidate) => candidate.active) ??
    candidates.find((candidate) => candidate.baseUrl === baseUrl) ??
    candidates.find((candidate) => candidate.key === "openrouter");

  const lineup = hosted ? (
    <HostedLineup status={status} provider={activeProvider} onModelChange={modelChanged} />
  ) : !status.reachable ? (
    <Text as="p" textRole="compact" tone="danger">
      Couldn't reach the Ollama host. Fix the URL above, then re-run the connection test.
    </Text>
  ) : (
    <OllamaLineup status={status} onModelChange={modelChanged} />
  );

  if (!onRoleSettingChange) return lineup;

  return (
    <Stack gap="4">
      <Stack
        aria-labelledby="role-lineup"
        className="border-border border-t pt-4"
        gap="control"
        render="section"
      >
        <div>
          <Text as="h3" id="role-lineup" textRole="heading">
            Choose a lineup model
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            Loomarr uses this model to plan channel suggestions. It must support tool calling.
          </Text>
        </div>
        {lineup}
      </Stack>
      {tmdbConfigured !== undefined && <TmdbNotice configured={tmdbConfigured} />}
      <VisionSection
        status={status}
        hosted={hosted}
        provider={activeProvider}
        active={visionChoiceOf(visionProvider, visionModel, hosted)}
        onChange={(choice) => {
          for (const [key, value] of visionChoiceSettings(choice)) onRoleSettingChange(key, value);
        }}
      />
    </Stack>
  );
};

export { AiModelSettings };
