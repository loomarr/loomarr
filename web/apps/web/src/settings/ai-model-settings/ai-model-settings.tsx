import * as systemApi from "@loomarr/api/endpoints/system";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { HostedModelPicker } from "@/components/loomarr/ai/hosted-model-picker";
import { ModelDiscover } from "@/components/loomarr/ai/model-discover";
import { ModelPicker } from "@/components/loomarr/ai/model-picker";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { useLoomarrEventListener } from "@/events/events-provider";

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
}: {
  provider?: string;
  baseUrl?: string;
  onBaseUrlChange?: (value: string) => void;
  onModelChange?: () => void;
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

  // Hosted (OpenAI-compatible): render the live model picker over the provider's /models.
  // Reacts to the LIVE provider, so it appears the instant the dropdown flips, before Save.
  if (isHosted) {
    const hosted = status.hosted ?? [];
    const activeProvider =
      hosted.find((candidate) => candidate.active) ??
      hosted.find((candidate) => candidate.baseUrl === baseUrl) ??
      hosted.find((candidate) => candidate.key === "openrouter");
    const testResult = unwrap(testProvider.data);
    const modelReady = Boolean(
      status.model && activeProvider?.models?.some((model) => model.id === status.model && model.tools),
    );
    return (
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
          providers={hosted}
          activeModel={status.model}
          busy={select.isPending}
          onSelect={(sel) => select.mutate({ data: sel })}
        />
      </div>
    );
  }

  if (!status.reachable) {
    return (
      <p className="text-onair-300 text-sm">
        Couldn't reach the Ollama host. Fix the URL above, then re-run the connection test.
      </p>
    );
  }

  const catalog = status.catalog ?? [];
  const discoverBody = unwrap(discover.data);
  const discovered = discoverBody?.models ?? undefined;
  // "source down" (HF unreachable / rate-limited) is NOT the same as "0 compatible" —
  // the BE flags it so the UI shows a browse link instead of a misleading empty state.
  const discoverFailed =
    (discover.data !== undefined && discover.data.status !== 200) || discoverBody?.sourceOk === false;

  return (
    <div className="flex flex-col gap-5">
      {(select.error ?? pull.error) != null && <ErrorState error={select.error ?? pull.error} />}
      {pullError && <ErrorState error={new Error(pullError)} />}

      {catalog.length > 0 ? (
        <ModelPicker
          catalog={catalog}
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
};

export { AiModelSettings };
