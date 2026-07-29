import { settingsApi, setupApi, systemApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Loader2 } from "lucide-react";
import { useState } from "react";
import { ErrorState, SettingsFields } from "@/components/loomarr";
import { Button } from "@/components/ui";
import { AiModelSettings, useSettingsEntries } from "@/settings";

// The wizard's AI onboarding (config-design §6: "AI — skippable; includes the §8.1 model
// picker"). It mirrors Settings → AI: the `ai` group's PROVIDER form (Ollama vs an
// OpenAI-compatible service — the URL + key fields reveal themselves for the hosted path via
// the registry's ShowWhen) sits above the model picker, and the picker reacts to the LIVE
// provider edit — so switching to "OpenAI-compatible" flips the surface before Save, exactly
// like Settings. The heavy lifting (installed picker, download, select/pull, hot-swap) is the
// shared AiModelSettings component, reused so the two never drift.
//
// Two seams the wizard needs that plain Settings doesn't:
//   1. Selecting a model must flip the wizard's `llm` checklist dot green — AiModelSettings
//      only invalidates its OWN status, so we pass onModelChange to also refresh setup-status.
//   2. The provider form saves through the same PATCH path the other checklist blocks use.
//
// Optional by design: skipping AI is neutral, never a red X (the ConnectionBlock wrapper owns
// that). Suggestions simply stay off until a model is chosen.
const AI_GROUP = "ai";

const WizardAiBlock = () => {
  const queryClient = useQueryClient();
  const entries = useSettingsEntries();
  const llm = systemApi.useSystemLlmStatus({ query: { retry: false } });
  const status = llm.data?.status === 200 ? llm.data.data : undefined;

  // Local edits for the provider fields, saved as a group like every other checklist block.
  const [edits, setEdits] = useState<Record<string, string>>({});
  const patch = settingsApi.useSettingsPatch({
    mutation: {
      onSuccess: async () => {
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: settingsApi.getSettingsListQueryKey() }),
          queryClient.invalidateQueries({ queryKey: setupApi.getSetupStatusQueryKey() }),
          queryClient.invalidateQueries({ queryKey: systemApi.getSystemLlmStatusQueryKey() }),
        ]);
        setEdits({});
      },
    },
  });
  const patchResults = patch.data?.status === 200 ? (patch.data.data.results ?? undefined) : undefined;

  // Essentials only (§6): advanced keys live in Settings. This is the provider dropdown +
  // the URL/key fields whose ShowWhen reveals them when provider = openai.
  const aiEntries = entries.filter((e) => e.group === AI_GROUP && !e.advanced);

  // The LIVE provider: an unsaved dropdown edit wins over the resolved value, so the picker
  // surface flips the instant you switch to OpenAI-compatible — before Save.
  const liveProvider =
    edits["llm.provider"] ?? aiEntries.find((e) => e.key === "llm.provider")?.value ?? status?.provider;

  // A model became active (or provider saved): flip the wizard's `llm` checklist dot green.
  const refreshSetupStatus = () =>
    queryClient.invalidateQueries({ queryKey: setupApi.getSetupStatusQueryKey() });

  const dirty = Object.keys(edits).length > 0;

  return (
    <div className="flex flex-col gap-4">
      {/* Provider choice + (for a hosted provider) its URL/key — the same fields Settings→AI
          shows, so the wizard offers the full Ollama-vs-external choice, not Ollama only. */}
      <SettingsFields
        entries={aiEntries}
        values={edits}
        onChange={(key, value) => setEdits((p) => ({ ...p, [key]: value }))}
        results={patchResults}
      />

      {patch.error != null && <ErrorState error={patch.error} />}

      {dirty && (
        <Button
          variant="outline"
          className="self-start"
          disabled={patch.isPending}
          onClick={() => patch.mutate({ data: { edits } })}
        >
          {patch.isPending ? "Saving…" : "Save provider"}
        </Button>
      )}

      {/* Auto-detect header — only meaningful on the local (Ollama) branch. */}
      {liveProvider !== "openai" &&
        (llm.isLoading ? (
          <p className="flex items-center gap-2 text-muted-foreground text-sm">
            <Loader2 className="size-4 animate-spin" aria-hidden />
            Looking for a local AI model…
          </p>
        ) : status?.reachable ? (
          <p className="flex items-center gap-2 text-lock text-sm">
            <CheckCircle2 className="size-4" aria-hidden />
            {status.model
              ? `Ollama detected: using ${status.model}.`
              : "Ollama detected. Pick a model below to turn on suggestions."}
          </p>
        ) : (
          <p className="text-muted-foreground text-sm">
            No local Ollama found. Start Ollama on this machine, or switch the provider above to an
            OpenAI-compatible service. You can skip this and add it later. Suggestions stay off until you do.
          </p>
        ))}

      {/* The real picker: local installed/download list, OR the hosted model picker — chosen
          by the LIVE provider. onModelChange flips the wizard's green dot when a model is set. */}
      <AiModelSettings provider={liveProvider} onModelChange={refreshSetupStatus} />
    </div>
  );
};

export { WizardAiBlock };
