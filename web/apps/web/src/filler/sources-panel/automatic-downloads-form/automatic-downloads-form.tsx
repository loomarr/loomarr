import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";
import { SourceAutomaticDownloadsInputMode } from "@loomarr/api/models/sourceAutomaticDownloadsInputMode";
import { formatRelative } from "@loomarr/core/format";
import { Action, Stack } from "@loomarr/design-system";
import { NumberField } from "@loomarr/ui";
import { useStore } from "@tanstack/react-form";
import { type ComponentRef, useRef, useState } from "react";
import { toast } from "sonner";
import { BrowserSelectControl } from "@/client-adapters";
import { useAppForm } from "@/lib/app-form";
import type { SourceActions } from "../use-source-actions";

type DownloadIntervalUnit = "minutes" | "hours" | "days";
type DownloadSchedulePreset = "6h" | "12h" | "daily" | "weekly" | "custom";

const downloadIntervalParts = (seconds: number): { amount: number; unit: DownloadIntervalUnit } => {
  if (seconds > 0 && seconds % 86400 === 0) return { amount: seconds / 86400, unit: "days" };
  if (seconds >= 3600 && seconds % 3600 === 0) return { amount: seconds / 3600, unit: "hours" };
  return { amount: Math.max(seconds / 60, 1), unit: "minutes" };
};

const downloadIntervalSeconds = (amount: number, unit: DownloadIntervalUnit): number =>
  Math.round(Math.max(amount, 1) * ({ days: 86400, hours: 3600, minutes: 60 } as const)[unit]);

const downloadIntervalMaximum = (unit: DownloadIntervalUnit): number =>
  ({ days: 7, hours: 168, minutes: 10080 })[unit];

const downloadSchedulePreset = (seconds: number): DownloadSchedulePreset => {
  if (seconds === 6 * 3600) return "6h";
  if (seconds === 12 * 3600) return "12h";
  if (seconds === 24 * 3600) return "daily";
  if (seconds === 7 * 24 * 3600) return "weekly";
  return "custom";
};

const downloadScheduleSeconds: Record<Exclude<DownloadSchedulePreset, "custom">, number> = {
  "6h": 6 * 3600,
  "12h": 12 * 3600,
  daily: 24 * 3600,
  weekly: 7 * 24 * 3600,
};

const MODE_OPTIONS = [
  { label: "Use automatic-download defaults", value: SourceAutomaticDownloadsInputMode.defaults },
  { label: "Use a different schedule", value: SourceAutomaticDownloadsInputMode.custom },
  { label: "Never download automatically", value: SourceAutomaticDownloadsInputMode.never },
] as const;

const SCHEDULE_OPTIONS: readonly { label: string; value: DownloadSchedulePreset }[] = [
  { label: "Every 6 hours", value: "6h" },
  { label: "Every 12 hours", value: "12h" },
  { label: "Daily", value: "daily" },
  { label: "Weekly", value: "weekly" },
  { label: "Custom", value: "custom" },
];

const UNIT_OPTIONS: readonly { label: string; value: DownloadIntervalUnit }[] = [
  { label: "minutes", value: "minutes" },
  { label: "hours", value: "hours" },
  { label: "days", value: "days" },
];

interface AutomaticDownloadsFormProps {
  source: FillerSourceDTO;
  actions: Pick<SourceActions, "updateSource" | "updating" | "updatingSourceID">;
}

type AutomaticDownloadsDraft = {
  mode: SourceAutomaticDownloadsInputMode;
  everySeconds: number;
  maxPerCheck: number;
};

// The schedule numbers only matter to a custom schedule; switching between defaults and never
// changes nothing else.
const sameDraft = (a: AutomaticDownloadsDraft, b: AutomaticDownloadsDraft): boolean =>
  a.mode === b.mode &&
  (a.mode !== SourceAutomaticDownloadsInputMode.custom ||
    (a.everySeconds === b.everySeconds && a.maxPerCheck === b.maxPerCheck));

// Per-source "Automatic downloads": follow the defaults, use a schedule of its own, or never. The
// draft is seeded from the source once, when the form mounts — remount per source (`key`) — so a
// background refetch cannot overwrite what the operator is mid-way through typing. Save stays off
// until the draft differs from what was last saved.
const AutomaticDownloadsForm = ({ source, actions }: AutomaticDownloadsFormProps) => {
  const [saved, setSaved] = useState<AutomaticDownloadsDraft>(() => ({
    mode: source.automaticDownloads?.mode ?? SourceAutomaticDownloadsInputMode.defaults,
    everySeconds: source.automaticDownloads?.everySeconds || 6 * 3600,
    maxPerCheck: source.automaticDownloads?.maxPerCheck ?? 10,
  }));
  const form = useAppForm({
    defaultValues: saved,
    onSubmit: ({ value }) => save(value),
  });
  const { mode, everySeconds, maxPerCheck } = useStore(form.store, (state) => state.values);
  const setMode = (next: SourceAutomaticDownloadsInputMode) => form.setFieldValue("mode", next);
  const setEverySeconds = (next: number) => form.setFieldValue("everySeconds", next);
  const setMaxPerCheck = (next: number) => form.setFieldValue("maxPerCheck", next);
  const [editingCustomSchedule, setEditingCustomSchedule] = useState(
    downloadSchedulePreset(saved.everySeconds) === "custom",
  );
  const [saveError, setSaveError] = useState<string>();
  const saveButton = useRef<ComponentRef<typeof Action>>(null);
  // The mode select's own trigger is the one control here that carries `aria-expanded`.
  const modeField = useRef<HTMLDivElement>(null);
  const automaticDownloads = source.automaticDownloads;
  if (!automaticDownloads) return null;
  const interval = downloadIntervalParts(everySeconds);
  const draft: AutomaticDownloadsDraft = { mode, everySeconds, maxPerCheck };
  const changed = !sameDraft(draft, saved);

  const save = ({ mode, everySeconds, maxPerCheck }: AutomaticDownloadsDraft) => {
    setSaveError(undefined);
    actions.updateSource(
      source.id,
      {
        enabled: source.enabled,
        automaticDownloads:
          mode === SourceAutomaticDownloadsInputMode.custom
            ? { mode: SourceAutomaticDownloadsInputMode.custom, everySeconds, maxPerCheck }
            : { mode },
      },
      {
        onSuccess: () => {
          setSaved({ mode, everySeconds, maxPerCheck });
          toast.success("Automatic downloads updated");
          // Save turns off once nothing is left to save, so focus goes back to the form.
          requestAnimationFrame(() =>
            modeField.current?.querySelector<HTMLElement>("[aria-expanded]")?.focus(),
          );
        },
        onError: (error) => {
          setSaveError(error.detail ?? "Automatic downloads couldn’t be saved. Try again.");
          requestAnimationFrame(() => saveButton.current?.focus());
        },
      },
    );
  };

  return (
    <Stack gap="control">
      <div>
        <p className="text-sm font-medium">Automatic downloads</p>
        <p className="text-muted-foreground mt-1 text-sm">{automaticDownloads.summary}</p>
        {automaticDownloads.nextCheckAt && (
          <p className="text-muted-foreground mt-1 text-xs">
            Next automatic check {formatRelative(automaticDownloads.nextCheckAt)}
          </p>
        )}
      </div>
      <div ref={modeField}>
        <BrowserSelectControl
          appearance="field"
          label="Automatic downloads for this source"
          labelHidden
          onValueChange={(value) => {
            setMode(value);
            if (value === SourceAutomaticDownloadsInputMode.custom) {
              setEditingCustomSchedule(downloadSchedulePreset(everySeconds) === "custom");
            }
          }}
          options={MODE_OPTIONS}
          value={mode}
        />
      </div>
      {mode === SourceAutomaticDownloadsInputMode.custom && (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Stack gap="1.5">
            <span className="text-sm">Look for new clips</span>
            <BrowserSelectControl
              appearance="field"
              label="Source check schedule"
              labelHidden
              onValueChange={(preset) => {
                setEditingCustomSchedule(preset === "custom");
                if (preset !== "custom") {
                  setEverySeconds(downloadScheduleSeconds[preset]);
                }
              }}
              options={SCHEDULE_OPTIONS}
              value={editingCustomSchedule ? "custom" : downloadSchedulePreset(everySeconds)}
            />
          </Stack>
          <Stack gap="1.5">
            <span className="text-sm">Add up to</span>
            <NumberField
              accessibilityLabel="Clips per source check"
              amount={maxPerCheck}
              onAmount={(amount) => setMaxPerCheck(Math.min(1000, Math.max(amount, 1)))}
            />
          </Stack>
          {editingCustomSchedule && (
            <Stack className="sm:col-span-2" gap="1.5">
              <span className="text-sm">Custom schedule</span>
              <Stack direction="row" gap="inline">
                <NumberField
                  accessibilityLabel="Source check interval"
                  amount={interval.amount}
                  onAmount={(amount) => {
                    const maximum = downloadIntervalMaximum(interval.unit);
                    setEverySeconds(downloadIntervalSeconds(Math.min(amount, maximum), interval.unit));
                  }}
                />
                <div className="w-28 shrink-0">
                  <BrowserSelectControl
                    appearance="field"
                    label="Source check interval unit"
                    labelHidden
                    onValueChange={(unit) => {
                      setEverySeconds(
                        downloadIntervalSeconds(
                          Math.min(interval.amount, downloadIntervalMaximum(unit)),
                          unit,
                        ),
                      );
                    }}
                    options={UNIT_OPTIONS}
                    value={interval.unit}
                  />
                </div>
              </Stack>
            </Stack>
          )}
        </div>
      )}
      <Action
        disabled={actions.updating || !changed}
        ref={saveButton}
        style={{ alignSelf: "flex-start" }}
        variant="outline"
        onPress={() => void form.handleSubmit()}
      >
        {actions.updatingSourceID === source.id ? "Saving…" : "Save automatic downloads"}
      </Action>
      {saveError && (
        <p role="alert" className="bg-destructive/10 text-destructive rounded-md px-3 py-2 text-sm">
          {saveError}
        </p>
      )}
    </Stack>
  );
};

export {
  AutomaticDownloadsForm,
  downloadIntervalMaximum,
  downloadIntervalParts,
  downloadIntervalSeconds,
  downloadSchedulePreset,
};
