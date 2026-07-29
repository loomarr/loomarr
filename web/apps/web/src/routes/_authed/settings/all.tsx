import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { AllSettingsTable } from "@/components/loomarr";
import { useSettingsEdits, useSettingsEntries } from "@/settings";

// Settings → All settings (config-design §5, V10) — the escape hatch.
//
// ⚠ Deliberately NOT a `SettingsPage`. Every other tab is an editor with a save bar; this one is
// a LOOKUP, and the difference is the point. Someone arrives here holding a literal key string —
// from a compose file, an env var, a log line — and asks "what is it set to, and which page owns
// it?". A page of humanized labels ("Job workers") cannot answer that, because nothing on screen
// matches `job.workers`. So the table shows raw keys, and the Group column says where to edit.
//
// This also replaced the old "Advanced" page, which had quietly become a dumping ground for
// whatever did not fit elsewhere. Advanced keys are still marked (the ADV chip) but they are no
// longer a *category* — they are an attribute of a key that lives on some other page.
const AllSettings = () => {
  const entries = useSettingsEntries();
  // The SAME cross-tab buffer every other Settings page stages into (V9). An edit made here and
  // saved from the bar is indistinguishable from one made on its home page — which is what lets
  // this table be the editor of last resort for keys whose page folded away.
  const { edits, setEdit } = useSettingsEdits();
  // Local rather than a search param, for now. A shareable link to "every playout key" is a
  // real want, but `?q=` here would be the only searchable Settings surface with one — worth
  // doing when there is a second, so the convention is set once rather than guessed.
  const [query, setQuery] = useState("");

  return (
    <div className="h-full overflow-auto p-6">
      <div className="mb-4">
        <h1 className="font-semibold text-xl">All settings</h1>
        <p className="mt-1 text-muted-foreground text-sm">
          Every key Loomarr knows about, searchable by name, group, or value. Edit a setting on the page that
          owns it. The Group column says which.
        </p>
      </div>
      <AllSettingsTable
        entries={entries}
        query={query}
        onQueryChange={setQuery}
        values={edits}
        onEdit={setEdit}
      />
    </div>
  );
};

const Route = createFileRoute("/_authed/settings/all")({
  component: AllSettings,
});

export { Route };
