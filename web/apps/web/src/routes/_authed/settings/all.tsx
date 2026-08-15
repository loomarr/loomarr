import { settingsApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { AllSettingsTable } from "@/components/loomarr";
import { useSettingsEdits, useSettingsEntries } from "@/settings";

// Settings → All settings (config-design §5, V10) — the escape hatch.
//
// ⚠ Deliberately NOT a `SettingsPage`. Someone arrives here holding a literal key string — from a
// compose file, an env var, or a log line — and asks "what is it set to, and which workflow owns
// it?" A page of humanized labels ("Job workers") cannot answer that, because nothing on screen
// matches `job.workers`. The table therefore keeps raw keys, edits them through the shared save
// buffer, and links each Group back to its workflow home.
//
// This also replaced the old "Advanced" page, which had quietly become a dumping ground for
// whatever did not fit elsewhere. Advanced keys are still marked (the ADV chip) but they are no
// longer a *category* — they are an attribute of a key that lives on some other page.
const AllSettings = () => {
  const queryClient = useQueryClient();
  const entries = useSettingsEntries();
  // The SAME cross-tab buffer every other Settings page stages into (V9). An edit made here and
  // saved from the bar is indistinguishable from one made on its home page — which is what lets
  // this table be the editor of last resort for keys whose page folded away.
  const { edits, setEdit } = useSettingsEdits();
  // Local rather than a search param, for now. A shareable link to "every playout key" is a
  // real want, but `?q=` here would be the only searchable Settings surface with one — worth
  // doing when there is a second, so the convention is set once rather than guessed.
  const [query, setQuery] = useState("");
  const refresh = () => queryClient.invalidateQueries({ queryKey: settingsApi.getSettingsListQueryKey() });
  const envOverride = settingsApi.useSettingsEnvOverride({ mutation: { onSuccess: refresh } });
  const clear = settingsApi.useSettingsClear({ mutation: { onSuccess: refresh } });

  return (
    <div className="h-full overflow-auto p-6">
      <div className="mb-4">
        <h1 className="font-semibold text-xl">All settings</h1>
        <p className="mt-1 text-muted-foreground text-sm">
          Every key Loomarr knows about, searchable by name, group, or value. Edit it here, or follow its
          Group to the workflow that owns it.
        </p>
      </div>
      <AllSettingsTable
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
  );
};

const Route = createFileRoute("/_authed/settings/all")({
  component: AllSettings,
});

export { Route };
