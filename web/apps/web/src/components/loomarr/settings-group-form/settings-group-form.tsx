import type { SettingEntry } from "@loomarr/api";
import { useForm } from "@tanstack/react-form";
import { CircleCheck, CircleX, Loader2 } from "lucide-react";
import { SettingsFields } from "@/components/loomarr/settings-fields";
import { Button } from "@/components/ui";
import { cn } from "@/lib";
import type { SettingsGroupFormProps } from "./settings-group-form.type";

// Only the fields the operator actually changed belong in a PATCH. This is a CORRECTNESS
// rule, not an optimization: a stored secret always reads back as "" (config-design §4 —
// never echoed), and an empty-string PATCH *clears* an optional key (§9). Submitting every
// field would therefore wipe every stored secret on save.
//
// Values are carried positionally (aligned with `entries`) rather than in a key→value map
// because TanStack Form reads a dot in a field name as a NESTED PATH — and every registry
// key is dotted ("library.url"), so `name="library.url"` would write {library:{url}} and
// never match the flat key the API expects. Indices keep the form agnostic to key shape.
const changedEntries = (values: string[], entries: SettingEntry[]): Record<string, string> => {
  const edits: Record<string, string> = {};
  entries.forEach((entry, i) => {
    const next = values[i] ?? "";
    if (next !== (entry.value ?? "")) edits[entry.key] = next;
  });
  return edits;
};

// SettingsGroupForm — one settings group as a form (config-design §5, §6). This is the
// ONLY settings form in the app: the wizard renders a group per step and Settings renders
// a group per page, both writing through the same PATCH /v1/settings path — "no parallel
// form system". Advanced keys hide behind a per-group toggle (§5), and the group's live
// check runs inline so the flow is configure → validate → save.
//
// The form owns its edit state (TanStack Form, §14) and hands `onSave` only the changed
// keys. It deliberately does NOT re-seed itself when `entries` change: a background
// refetch must never overwrite what the operator is typing. A caller that wants a fresh
// baseline after a successful save remounts it with a `key` derived from the saved entries.
const SettingsGroupForm = ({
  entries,
  onSave,
  saving = false,
  results,
  onTest,
  testing = false,
  testOk,
  testHint,
  className,
}: SettingsGroupFormProps) => {
  const form = useForm({
    defaultValues: { values: entries.map((entry) => entry.value ?? "") },
    onSubmit: ({ value }) => onSave(changedEntries(value.values, entries)),
  });

  return (
    <form
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        form.handleSubmit();
      }}
      className={cn("flex flex-col gap-5", className)}
    >
      {/* The field rendering is shared with the Settings pages via SettingsFields; what
          differs here is the save UNIT — the wizard commits one group per step. */}
      <form.Field name="values">
        {(field) => (
          <SettingsFields
            entries={entries}
            values={Object.fromEntries(entries.map((e, i) => [e.key, field.state.value[i] ?? e.value ?? ""]))}
            onChange={(key, v) => {
              const i = entries.findIndex((e) => e.key === key);
              const next = [...field.state.value];
              next[i] = v;
              field.handleChange(next);
            }}
            results={results}
          />
        )}
      </form.Field>

      {(testHint || testOk !== undefined) && !testing && (
        <p
          className={cn("flex items-center gap-1.5 text-sm", testOk ? "text-lock" : "text-onair-300")}
          role="status"
        >
          {testOk ? (
            <CircleCheck className="size-4" aria-hidden />
          ) : (
            <CircleX className="size-4" aria-hidden />
          )}
          {testHint ?? (testOk ? "Connection OK" : "Connection failed")}
        </p>
      )}

      <div className="flex items-center gap-2">
        {onTest && (
          <Button type="button" variant="outline" onClick={onTest} disabled={testing || saving}>
            {testing && <Loader2 className="animate-spin" aria-hidden />}
            {testing ? "Testing…" : "Test connection"}
          </Button>
        )}
        <Button type="submit" disabled={saving || testing}>
          {saving && <Loader2 className="animate-spin" aria-hidden />}
          {saving ? "Saving…" : "Save"}
        </Button>
      </div>
    </form>
  );
};

export { changedEntries, SettingsGroupForm };
