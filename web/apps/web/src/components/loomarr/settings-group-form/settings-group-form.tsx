import { CircleCheck, CircleX, Loader2 } from "lucide-react";
import { useState } from "react";
import { SettingField } from "@/components/loomarr/setting-field";
import { Button } from "@/components/ui";
import { cn } from "@/lib";
import type { SettingsGroupFormProps } from "./settings-group-form.type";

// SettingsGroupForm — one settings group as a form (config-design §5, §6). This is the
// ONLY settings form in the app: the wizard renders a group per step and Settings
// renders a group per page, both writing through the same PATCH /v1/settings path —
// "no parallel form system". Advanced keys hide behind a per-group toggle (§5), and the
// group's live check runs inline so the flow is configure → validate → save.
const SettingsGroupForm = ({
  entries,
  values,
  onChange,
  onSave,
  saving = false,
  results,
  onTest,
  testing = false,
  testOk,
  testHint,
  className,
}: SettingsGroupFormProps) => {
  const [showAdvanced, setShowAdvanced] = useState(false);
  const advancedCount = entries.filter((e) => e.advanced).length;
  const visible = entries.filter((e) => !e.advanced || showAdvanced);
  const resultFor = (key: string) => results?.find((r) => r.key === key);

  return (
    <div className={cn("flex flex-col gap-5", className)}>
      <div className="flex flex-col gap-5">
        {visible.map((entry) => (
          <SettingField
            key={entry.key}
            entry={entry}
            value={values[entry.key] ?? ""}
            onChange={(v) => onChange(entry.key, v)}
            result={resultFor(entry.key)}
          />
        ))}
      </div>

      {advancedCount > 0 && (
        <button
          type="button"
          onClick={() => setShowAdvanced((v) => !v)}
          className="w-fit text-muted-foreground text-sm underline-offset-4 hover:text-foreground hover:underline"
        >
          {showAdvanced ? "Hide advanced" : `Show advanced (${advancedCount})`}
        </button>
      )}

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
          <Button variant="outline" onClick={onTest} disabled={testing || saving}>
            {testing && <Loader2 className="animate-spin" aria-hidden />}
            {testing ? "Testing…" : "Test connection"}
          </Button>
        )}
        {onSave && (
          <Button onClick={onSave} disabled={saving || testing}>
            {saving && <Loader2 className="animate-spin" aria-hidden />}
            {saving ? "Saving…" : "Save"}
          </Button>
        )}
      </div>
    </div>
  );
};

export { SettingsGroupForm };
