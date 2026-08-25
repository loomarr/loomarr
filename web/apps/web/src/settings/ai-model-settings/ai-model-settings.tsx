import * as systemApi from "@loomarr/api/endpoints/system";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useState } from "react";
import { HostedModelPicker } from "@/components/loomarr/ai/hosted-model-picker";
import { ModelDiscover } from "@/components/loomarr/ai/model-discover";
import { ModelPicker } from "@/components/loomarr/ai/model-picker";
import { CollapsibleSection } from "@/components/loomarr/feedback/collapsible-section";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { useLoomarrEventListener } from "@/events/events-provider";

type RoleOption = {
  id: string;
  label: string;
  detail: string;
};

const RolePicker = ({
  title,
  description,
  options,
  active,
  onSelect,
}: {
  title: string;
  description: string;
  options: RoleOption[];
  active: string;
  onSelect: (id: string) => void;
}) => {
  const [expanded, setExpanded] = useState(false);
  const guided = options.slice(0, 3);
  const current = options.find(
    (option) => option.id === active && !guided.some((item) => item.id === active),
  );
  const shown = expanded ? options : current ? [...guided, current] : guided;

  return (
    <section
      aria-labelledby={`role-${title.toLowerCase()}`}
      className="flex flex-col gap-2 rounded-md border border-border p-3"
    >
      <div>
        <h3 id={`role-${title.toLowerCase()}`} className="font-medium text-sm">
          {title}
        </h3>
        <p className="text-muted-foreground text-sm">{description}</p>
      </div>
      <div className="grid gap-2">
        {shown.map((option) => {
          const selected = option.id === active;
          return (
            <Button
              key={option.id}
              type="button"
              variant={selected ? "secondary" : "outline"}
              className="h-auto min-h-10 justify-start whitespace-normal px-3 py-2 text-left"
              onClick={() => onSelect(option.id)}
              disabled={selected}
            >
              <span>
                <span className="block font-medium">{option.label}</span>
                <span className="block font-normal text-muted-foreground text-xs">{option.detail}</span>
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

// The §8.1 model picker, wired. Selecting hot-swaps the running suggester (no restart),
// which is why this sits beside the AI settings form rather than inside it: the form
// writes llm.* through PATCH, while select/pull are actions with their own lifecycles.
//
// Local Ollama gets two surfaces: the INSTALLED list (fit-ranked, tool-capable, select or
// grey-out) and a compatible-to-DOWNLOAD list (popular HF GGUF models sized against this
// machine's VRAM, best-first) — because Ollama ships no "what can I download" API, the
// catalog is what you've pulled plus a machine-ranked browse, never a hardcoded list. A
// hosted (OpenAI-compatible) provider instead gets a live model picker over its /models.
//
// Pull progress exists ONLY on the SSE stream (`llm_pull` frames), so it comes through
// the shared event fan-out — the same reason the Suggest workspace does.
//
// `provider` is the LIVE (possibly unsaved) provider from the settings form, so the right
// surface appears the instant the dropdown flips — it doesn't wait for Save, matching how
// the URL/key fields already react live.
// onModelChange fires after a successful model select OR a completed pull — the seam a host
// (e.g. the wizard) uses to refresh things this component doesn't own, like the setup-status
// checklist whose `llm` verdict flips green once a model is active. Settings passes nothing
// (its own status invalidation is enough); the wizard passes a setup-status invalidation.
const AiModelSettings = ({
  provider,
  baseUrl,
  onBaseUrlChange,
  onModelChange,
  visionProvider = "inherit",
  visionModel = "",
  transcriptionProvider = "whisper",
  transcriptionModel = "openai/whisper-large-v3",
  onRoleSettingChange,
}: {
  provider?: string;
  baseUrl?: string;
  onBaseUrlChange?: (value: string) => void;
  onModelChange?: () => void;
  visionProvider?: string;
  visionModel?: string;
  transcriptionProvider?: string;
  transcriptionModel?: string;
  onRoleSettingChange?: (key: string, value: string) => void;
}) => {
  const queryClient = useQueryClient();
  const [pulling, setPulling] = useState<{ tag: string; percent?: number }>();
  // A failed download must SAY so. Clearing the progress on error looked identical to
  // success, leaving the operator to notice the row still said "Download".
  const [pullError, setPullError] = useState<string>();

  const isHosted = provider === "openai";
  // Status carries BOTH the local catalog and the hosted-provider catalog, so it's always
  // fetched — the branch below picks which surface to render from the same payload.
  const llm = systemApi.useSystemLlmStatus({ query: { retry: false } });
  const status = unwrap(llm.data);
  const openRouterBase = status?.hosted?.find((hosted) => hosted.key === "openrouter")?.baseUrl;

  // The probe already owns the blessed provider's canonical API base. When someone chooses the
  // hosted path on a blank form, stage that fact into the same edit buffer as the visible field.
  // Never overwrite a non-empty URL: that is the Custom path and remains fully operator-owned.
  useEffect(() => {
    if (isHosted && baseUrl === "" && openRouterBase && onBaseUrlChange) {
      onBaseUrlChange(openRouterBase);
    }
  }, [baseUrl, isHosted, onBaseUrlChange, openRouterBase]);
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: systemApi.getSystemLlmStatusQueryKey() });
  // A model became active: refresh our own status AND notify the host (setup-status, etc.).
  const modelChanged = () => {
    invalidate();
    onModelChange?.();
  };

  const select = systemApi.useSystemLlmSelect({ mutation: { onSuccess: modelChanged } });
  const testProvider = systemApi.useSystemLlmTest();
  const pull = systemApi.useSystemLlmPull();

  // The compatible-to-download list: the BE ranks popular HF models against this
  // machine's VRAM. Fetched only on the Ollama branch (a hosted provider downloads
  // nothing); a pull success invalidates it so a freshly-pulled model leaves the list.
  const discover = systemApi.useSystemLlmDiscover({ query: { retry: false, enabled: !isHosted } });

  useLoomarrEventListener({
    onLlmPull: (e) => {
      if (!e.model) return;
      if (e.status === "error") {
        setPullError(e.error ?? `Downloading ${e.model} failed.`);
        setPulling(undefined);
        return;
      }
      // A successful terminal frame refreshes both surfaces: the freshly-pulled model
      // appears in the installed list and leaves the download list, no reload needed.
      if (e.status === "success") {
        setPulling(undefined);
        void invalidate();
        void queryClient.invalidateQueries({
          queryKey: systemApi.getSystemLlmDiscoverQueryKey(),
        });
        return;
      }
      setPulling({ tag: e.model, percent: e.percent && e.percent >= 0 ? e.percent : undefined });
    },
  });

  const startPull = (model: string) => {
    setPullError(undefined);
    setPulling({ tag: model });
    pull.mutate({ data: { model } });
  };

  if (llm.error) return <ErrorState error={llm.error} onRetry={() => llm.refetch()} />;
  if (!status) return <p className="text-muted-foreground text-sm">Checking your AI provider…</p>;

  const hosted = status.hosted ?? [];
  const activeProvider =
    hosted.find((candidate) => candidate.active) ??
    hosted.find((candidate) => candidate.baseUrl === baseUrl) ??
    hosted.find((candidate) => candidate.key === "openrouter");
  const catalog = status.catalog ?? [];
  const lineupCatalog = catalog.filter((model) => model.tools);
  const discoverBody = unwrap(discover.data);
  const discovered = discoverBody?.models ?? undefined;
  const discoverFailed =
    (discover.data !== undefined && discover.data.status !== 200) || discoverBody?.sourceOk === false;

  let lineup: ReactNode;
  if (isHosted) {
    const testResult = unwrap(testProvider.data);
    const modelReady = Boolean(
      status.model && activeProvider?.models?.some((model) => model.id === status.model && model.tools),
    );
    const lineupProviders = hosted.map((candidate) => ({
      ...candidate,
      models: candidate.models?.filter((model) => model.tools),
    }));
    lineup = (
      <div className="flex flex-col gap-3">
        {select.error != null && <ErrorState error={select.error} />}
        {activeProvider?.keyConfigured && (
          <div className="flex flex-wrap items-center gap-3">
            <Button
              variant="outline"
              disabled={testProvider.isPending}
              onClick={() =>
                testProvider.mutate({
                  data: {
                    provider: activeProvider.key,
                    ...(activeProvider.key === "custom" ? { baseUrl: activeProvider.baseUrl } : {}),
                  },
                })
              }
            >
              {testProvider.isPending ? "Testing credentials…" : "Test provider credentials"}
            </Button>
            {testResult?.ok && (
              <p role="status" className="text-lock text-sm">
                {activeProvider.label} credentials authorized.{" "}
                {modelReady
                  ? `${status.model} is ready for lineup suggestions.`
                  : "Choose a tool-capable lineup model to finish AI setup."}
              </p>
            )}
            {testResult && !testResult.ok && (
              <p role="status" className="text-onair-300 text-sm">
                {testResult.error ?? `${activeProvider.label} rejected these credentials.`}
              </p>
            )}
          </div>
        )}
        <HostedModelPicker
          providers={lineupProviders}
          activeModel={status.model}
          busy={select.isPending}
          onSelect={(sel) => select.mutate({ data: sel })}
        />
      </div>
    );
  } else if (!status.reachable) {
    lineup = (
      <p className="text-onair-300 text-sm">
        Couldn't reach the Ollama host. Fix the URL above, then re-run the connection test.
      </p>
    );
  } else {
    lineup = (
      <div className="flex flex-col gap-5">
        {(select.error ?? pull.error) != null && <ErrorState error={select.error ?? pull.error} />}
        {pullError && <ErrorState error={new Error(pullError)} />}
        {lineupCatalog.length > 0 ? (
          <ModelPicker
            catalog={lineupCatalog}
            active={status.model}
            gpuName={status.gpuName}
            vramGiB={status.vramGiB}
            busy={select.isPending}
            pulling={pulling}
            onSelect={(model) => select.mutate({ data: { model } })}
            onPull={startPull}
          />
        ) : (
          <p className="text-muted-foreground text-sm">
            No tool-capable models installed yet. Download one below to get started.
          </p>
        )}
        <ModelDiscover
          results={discovered}
          loading={discover.isFetching}
          sourceError={discoverFailed}
          pulling={pulling}
          onPull={startPull}
        />
      </div>
    );
  }

  if (!onRoleSettingChange) return lineup;

  const activeLineup = isHosted
    ? activeProvider?.models?.find((model) => model.id === status.model)
    : catalog.find((model) => model.tag === status.model);
  const visionOptions: RoleOption[] = [
    {
      id: "inherit",
      label: "Reuse the text-role choice",
      detail: activeLineup?.vision
        ? "This model advertises image input, so no separate vision model is needed."
        : "Inherits the text role. Choose a vision model below when that model cannot see images.",
    },
    ...catalog
      .filter((model) => model.vision)
      .map((model) => ({
        id: `ollama:${model.tag}`,
        label: model.label,
        detail: `Local Ollama · ${model.fit.replace("_", " ")} · ${model.why}`,
      })),
    ...(isHosted && activeProvider?.keyConfigured
      ? (activeProvider?.models ?? [])
          .filter((model) => model.vision)
          .map((model) => ({
            id: `hosted:${model.id}`,
            label: model.label,
            detail: `${activeProvider?.label ?? "Hosted"} · vision capable`,
          }))
      : []),
  ];
  const activeVision =
    visionProvider === "inherit" && visionModel === ""
      ? "inherit"
      : visionProvider === "ollama" || (!isHosted && visionProvider === "inherit")
        ? `ollama:${visionModel}`
        : `hosted:${visionModel}`;
  const transcriptionOptions: RoleOption[] = [
    {
      id: "whisper",
      label: "Bundled local Whisper",
      detail: "Default. Runs locally and does not use hosted credits.",
    },
    ...(isHosted && activeProvider?.keyConfigured
      ? (activeProvider?.models ?? [])
          .filter((model) => model.transcription)
          .map((model) => ({
            id: `hosted:${model.id}`,
            label: model.label,
            detail: `${activeProvider?.label ?? "Hosted"} · timed speech-to-text`,
          }))
      : []),
  ];
  const activeTranscription = transcriptionProvider === "hosted" ? `hosted:${transcriptionModel}` : "whisper";

  return (
    <div className="flex flex-col gap-4">
      <section
        aria-labelledby="automatic-model-policy"
        className="rounded-lg border border-border bg-static-900 p-5"
      >
        <h2 id="automatic-model-policy" className="font-display font-semibold text-xl">
          Automatic model policy
        </h2>
        <p className="mt-1 text-muted-foreground text-sm">
          Loomarr routes lineup planning, filler text, frames, video, and transcription independently using
          the last certified compatibility and quality policy. You do not need to maintain a model matrix.
        </p>
        <p className="mt-3 text-sm">
          {status.model
            ? `Current lineup route: ${status.model}. Existing manual choices remain unverified overrides until a certified policy replaces them.`
            : "No lineup route is active yet. Connect an AI provider; Loomarr will keep filler work held until a compatible route is available."}
        </p>
      </section>
      <section
        aria-labelledby="role-lineup"
        className="flex flex-col gap-3 rounded-md border border-border p-3"
      >
        <div>
          <h3 id="role-lineup" className="font-medium text-sm">
            Provider and current model
          </h3>
          <p className="text-muted-foreground text-sm">
            Connect the AI service Loomarr can use now. Certified automatic routes will replace this
            unverified choice when available.
          </p>
        </div>
        {lineup}
      </section>
      <CollapsibleSection
        title="Advanced model overrides"
        description="Replace individual filler routes. Overrides are recorded as unverified and never inherit certification."
      >
        <div className="flex flex-col gap-4">
          <RolePicker
            title="Vision"
            description="Reads sampled filler frames. It inherits the lineup model unless you choose a vision-capable model."
            options={visionOptions}
            active={activeVision}
            onSelect={(id) => {
              if (id === "inherit") {
                onRoleSettingChange("filler.vision.provider", "inherit");
                onRoleSettingChange("filler.vision.model", "");
              } else {
                const [kind, ...parts] = id.split(":");
                onRoleSettingChange("filler.vision.provider", kind === "ollama" ? "ollama" : "inherit");
                onRoleSettingChange("filler.vision.model", parts.join(":"));
              }
            }}
          />
          <RolePicker
            title="Transcription"
            description="Creates timed speech segments. Bundled Whisper is the local default; hosted choices use the same active provider credential."
            options={transcriptionOptions}
            active={activeTranscription}
            onSelect={(id) => {
              if (id === "whisper") {
                onRoleSettingChange("filler.transcribe.provider", "whisper");
              } else {
                onRoleSettingChange("filler.transcribe.provider", "hosted");
                onRoleSettingChange("filler.transcribe.model", id.slice("hosted:".length));
              }
            }}
          />
        </div>
      </CollapsibleSection>
    </div>
  );
};

export { AiModelSettings };
