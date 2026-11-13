import * as settingsApi from "@loomarr/api/endpoints/settings";
import { invalidateSettings } from "@loomarr/core/events";
import { Stack } from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { AdvancedSettingsTable } from "@/components/loomarr/settings/advanced-settings-table";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { useSettingsEdits } from "@/settings/settings-edits";
import { announceSettingsChange, announceSettingsFailure } from "@/settings/settings-feedback";
import { useSettingsEntries } from "@/settings/use-settings-entries";

// Settings → Advanced settings (config-design §5) — the raw registry escape hatch.
//
// ⚠ Deliberately NOT a `SettingsPage`. Someone arrives here holding a literal key string — from a
// compose file, an env var, or a log line — and asks "what is it set to, and which workflow owns
// it?" A page of humanized labels ("Job workers") cannot answer that, because nothing on screen
// matches `job.workers`. The table therefore keeps raw keys, edits them through the shared save
// buffer, and links each Group back to its workflow home.
//
// It is not an "Advanced" page: such a page becomes a dumping ground for whatever does not fit
// elsewhere. Advanced keys are marked (the ADV chip) but they are not a *category* — they are an
// attribute of a key that lives on some other page.
const AdvancedSettings = () => {
  const queryClient = useQueryClient();
  const entries = useSettingsEntries();
  // The SAME cross-tab buffer every other Settings page stages into (V9). An edit made here and
  // saved from the bar is indistinguishable from one made on its home page — which is what lets
  // this table be the editor of last resort for keys with no other page.
  const { edits, setEdit } = useSettingsEdits();
  // Local rather than a search param, for now. A shareable link to "every playout key" is a
  // real want, but `?q=` here would be the only searchable Settings surface with one — worth
  // doing when there is a second, so the convention is set once rather than guessed.
  const [query, setQuery] = useState("");
  const refresh = () => invalidateSettings(queryClient);
  const envOverride = settingsApi.useSettingsEnvOverride({
    mutation: {
      onSuccess: async (_response, { data }) => {
        await refresh();
        announceSettingsChange(data.enabled ? "Now editable here" : "Handed back to the environment");
      },
      onError: (error) => announceSettingsFailure(error, "Couldn't change who manages this setting"),
    },
  });
  const clear = settingsApi.useSettingsClear({
    mutation: {
      onSuccess: async () => {
        await refresh();
        announceSettingsChange("Setting cleared");
      },
      onError: (error) => announceSettingsFailure(error, "Couldn't clear this setting"),
    },
  });

  return (
    <Stack className="h-full min-h-0">
      <PageHeader
        title="Advanced settings"
        description="Every setting key Loomarr knows about. Search by key, group, or value, or follow its owner to the usual place to change it."
      />
      <div className="min-h-0 flex-1 overflow-auto p-6">
        <AdvancedSettingsTable
          entries={entries}
          query={query}
          onQueryChange={setQuery}
          values={edits}
          onEdit={setEdit}
          onEnvOverride={(key, enabled) => envOverride.mutate({ key, data: { enabled } })}
          onClear={(entry) => {
            if (
              entry.secret &&
              !window.confirm(
                `Clear ${entry.label || entry.key}? The integration will stop working until it is set again.`,
              )
            )
              return;
            clear.mutate({ key: entry.key });
          }}
        />
      </div>
    </Stack>
  );
};

// no-loader: reads only the settings list the Settings layout's loader ensured
const Route = createFileRoute("/_authed/settings/advanced")({
  component: AdvancedSettings,
});

export { Route };
