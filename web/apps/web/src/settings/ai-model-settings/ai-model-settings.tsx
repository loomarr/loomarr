import * as setupApi from "@loomarr/api/endpoints/setup";
import * as systemApi from "@loomarr/api/endpoints/system";
import type { HostedProviderView } from "@loomarr/api/models/hostedProviderView";
import type { SettingKey } from "@loomarr/api/models/settingKey";
import type { SystemLLMStatus } from "@loomarr/api/models/systemLLMStatus";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { HostedModelPicker } from "@/components/loomarr/ai/hosted-model-picker";
import { ModelDiscover } from "@/components/loomarr/ai/model-discover";
import { ModelPicker } from "@/components/loomarr/ai/model-picker";
import { CollapsibleSection } from "@/components/loomarr/feedback/collapsible-section";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button, buttonVariants } from "@/components/ui/button";
import { useModelPull } from "@/settings/use-model-pull";
import {
  REUSE_LINEUP,
  sameVisionChoice,
  type VisionChoice,
  visionChoiceOf,
  visionChoiceSettings,
} from "@/settings/vision-choice";

// A hosted provider's lineup: test its credentials, then pick a tool-capable model from its
// live /models list.
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
  const testProvider = systemApi.useSystemLlmTest();
  const testResult = unwrap(testProvider.data);
  const modelReady = Boolean(
    status.model && provider?.models?.some((model) => model.id === status.model && model.tools),
  );
  const providers = provider
    ? [{ ...provider, models: provider.models?.filter((model) => model.tools) }]
    : [];

  return (
    <div className="flex flex-col gap-3">
      {select.error != null && <ErrorState error={select.error} />}
      {provider?.keyConfigured && (
        <div className="flex flex-wrap items-center gap-3">
          <Button
            variant="outline"
            disabled={testProvider.isPending}
            onClick={() =>
              testProvider.mutate({
                data: {
                  provider: provider.key,
                  ...(provider.key === "custom" ? { baseUrl: provider.baseUrl } : {}),
                },
              })
            }
          >
            {testProvider.isPending ? "Testing credentials…" : "Test provider credentials"}
          </Button>
          {testResult?.ok && (
            <p role="status" className="text-lock text-sm">
              {provider.label} credentials authorized.{" "}
              {modelReady
                ? `${status.model} is selected as the lineup model.`
                : "Choose a tool-capable lineup model to finish AI setup."}
            </p>
          )}
          {testResult && !testResult.ok && (
            <p role="status" className="text-onair-300 text-sm">
              {testResult.error ?? `${provider.label} rejected these credentials.`}
            </p>
          )}
        </div>
      )}
      <HostedModelPicker
        providers={providers}
        activeModel={status.model}
        busy={select.isPending}
        onSelect={(sel) => select.mutate({ data: sel })}
      />
    </div>
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
    <div className="flex flex-col gap-5">
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
        <p className="text-muted-foreground text-sm">
          No tool-capable models installed yet. Download one below to get started.
        </p>
      )}
      <ModelDiscover
        results={discoverBody?.models ?? undefined}
        loading={discover.isFetching}
        sourceError={discoverFailed}
        pulling={pull.pulling}
        onPull={pull.start}
      />
    </div>
  );
};

const TmdbNotice = ({ configured }: { configured: boolean }) => (
  <section className="border-border bg-muted/40 flex flex-wrap items-center justify-between gap-3 rounded-md border p-3">
    <div>
      <p className="text-sm font-medium">
        {configured ? "TMDB key configured" : "TMDB grounding is still needed"}
      </p>
      <p className="text-muted-foreground text-sm">
        {configured
          ? "Loomarr can use TMDB to ground channel suggestions in real titles."
          : "Your AI connection is separate. Connect TMDB before building a channel from a description."}
      </p>
    </div>
    {!configured && (
      <Link
        to="/settings/connections"
        search={{ focus: "tmdb" }}
        className={buttonVariants({ variant: "outline", size: "sm" })}
      >
        Connect TMDB
      </Link>
    )}
  </section>
);

type VisionOption = { choice: VisionChoice; label: string; detail: string };

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
    <section
      aria-labelledby="role-vision"
      className="border-border flex flex-col gap-2 rounded-md border p-3"
    >
      <div>
        <h3 id="role-vision" className="text-sm font-medium">
          Vision
        </h3>
        <p className="text-muted-foreground text-sm">
          Reads sampled filler frames. It inherits the lineup model unless you choose a vision-capable model.
        </p>
      </div>
      <div className="grid gap-2">
        {shown.map((option) => {
          const selected = sameVisionChoice(option.choice, active);
          return (
            <Button
              key={`${option.choice.provider}:${option.choice.model}`}
              type="button"
              variant={selected ? "secondary" : "outline"}
              className="h-auto min-h-10 justify-start px-3 py-2 text-left whitespace-normal"
              onClick={() => onSelect(option.choice)}
              disabled={selected}
            >
              <span>
                <span className="block font-medium">{option.label}</span>
                <span className="text-muted-foreground block text-xs font-normal">{option.detail}</span>
              </span>
            </Button>
          );
        })}
      </div>
      {options.length > 3 && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="self-start"
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? "Show guided choices" : `Show all ${options.length} choices`}
        </Button>
      )}
    </section>
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
    <CollapsibleSection
      title="Advanced vision model"
      description="Choose a separate model only when the lineup model cannot read clip frames."
    >
      <div className="flex flex-col gap-4">
        <VisionPicker options={options} active={active} onSelect={onChange} />
      </div>
    </CollapsibleSection>
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
  if (!status) return <p className="text-muted-foreground text-sm">Checking your AI provider…</p>;

  const hosted = provider === "openai";
  const candidates = status.hosted ?? [];
  const activeProvider =
    candidates.find((candidate) => candidate.active) ??
    candidates.find((candidate) => candidate.baseUrl === baseUrl) ??
    candidates.find((candidate) => candidate.key === "openrouter");

  const lineup = hosted ? (
    <HostedLineup status={status} provider={activeProvider} onModelChange={modelChanged} />
  ) : !status.reachable ? (
    <p className="text-onair-300 text-sm">
      Couldn't reach the Ollama host. Fix the URL above, then re-run the connection test.
    </p>
  ) : (
    <OllamaLineup status={status} onModelChange={modelChanged} />
  );

  if (!onRoleSettingChange) return lineup;

  return (
    <div className="flex flex-col gap-4">
      <section aria-labelledby="role-lineup" className="border-border flex flex-col gap-3 border-t pt-4">
        <div>
          <h3 id="role-lineup" className="text-base font-semibold">
            Choose a lineup model
          </h3>
          <p className="text-muted-foreground text-sm">
            Loomarr uses this model to plan channel suggestions. It must support tool calling.
          </p>
        </div>
        {lineup}
      </section>
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
    </div>
  );
};

export { AiModelSettings };
