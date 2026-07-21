import { systemApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { ErrorState, ModelPicker } from "@/components/loomarr";
import { useLoomarrEventListener } from "@/events";

// The §8.1 model picker, wired. Selecting hot-swaps the running suggester (no restart),
// which is why this sits beside the AI settings form rather than inside it: the form
// writes llm.* through PATCH, while select/pull are actions with their own lifecycles.
//
// Pull progress exists ONLY on the SSE stream (`llm_pull` frames), so it comes through
// the shared event fan-out — the same reason the Suggest workspace does.
const AiModelSettings = () => {
  const queryClient = useQueryClient();
  const [pulling, setPulling] = useState<{ tag: string; percent?: number }>();
  // A failed download must SAY so. Clearing the progress on error looked identical to
  // success, leaving the operator to notice the row still said "Download".
  const [pullError, setPullError] = useState<string>();

  const llm = systemApi.useSystemLlmStatus({ query: { retry: false } });
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: systemApi.getSystemLlmStatusQueryKey() });

  const select = systemApi.useSystemLlmSelect({ mutation: { onSuccess: invalidate } });
  const pull = systemApi.useSystemLlmPull();

  useLoomarrEventListener({
    onLlmPull: (e) => {
      if (!e.model) return;
      if (e.status === "error") {
        setPullError(e.error ?? `Downloading ${e.model} failed.`);
        setPulling(undefined);
        return;
      }
      // A successful terminal frame refreshes the catalog, so the row flips from
      // "Download" to "Use this" without the operator reloading.
      if (e.status === "success") {
        setPulling(undefined);
        void invalidate();
        return;
      }
      // `percent` is computed by the BE and present from the first "starting" frame;
      // deriving it from completed/total here would show nothing until bytes flow, and
      // -1 is the failure sentinel rather than a real percentage.
      setPulling({ tag: e.model, percent: e.percent && e.percent >= 0 ? e.percent : undefined });
    },
  });

  if (llm.error) return <ErrorState error={llm.error} onRetry={() => llm.refetch()} />;
  const status = llm.data?.status === 200 ? llm.data.data : undefined;
  if (!status) return <p className="text-muted-foreground text-sm">Checking your AI provider…</p>;

  // Local Ollama gets the fit-ranked catalog; a hosted provider is configured through the
  // settings form + its own key test, so there is no local catalog to show.
  if (!status.local) {
    return (
      <p className="text-muted-foreground text-sm">
        Using the hosted provider <strong className="text-foreground">{status.provider}</strong>
        {status.model ? ` with ${status.model}` : ""}. Switch to Ollama above to pick a local model.
      </p>
    );
  }

  if (!status.reachable) {
    return (
      <p className="text-onair-300 text-sm">
        Couldn't reach the Ollama host. Fix the URL above, then re-run the connection test.
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      {(select.error ?? pull.error) != null && <ErrorState error={select.error ?? pull.error} />}
      {pullError && <ErrorState error={new Error(pullError)} />}
      <ModelPicker
        catalog={status.catalog ?? []}
        active={status.model}
        gpuName={status.gpuName}
        vramGiB={status.vramGiB}
        busy={select.isPending}
        pulling={pulling}
        onSelect={(model) => select.mutate({ data: { model } })}
        onPull={(model) => {
          setPullError(undefined);
          setPulling({ tag: model });
          pull.mutate({ data: { model } });
        }}
      />
    </div>
  );
};

export { AiModelSettings };
