import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";
import { SourceAutomaticDownloadsInputMode } from "@loomarr/api/models/sourceAutomaticDownloadsInputMode";
import { formatRelative } from "@loomarr/core/format";
import { Action } from "@loomarr/design-system";
import { NumberField } from "@loomarr/ui";
import { type ComponentRef, useRef, useState } from "react";
import { toast } from "sonner";
import { BrowserSelectControl } from "@/client-adapters";
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

// Per-source "Automatic downloads": follow the defaults, use a schedule of its own, or never. The
// draft is seeded from the source once, when the form mounts — remount per source (`key`) — so a
// background refetch cannot overwrite what the operator is mid-way through typing.
const AutomaticDownloadsForm = ({ source, actions }: AutomaticDownloadsFormProps) => {
  const [mode, setMode] = useState<SourceAutomaticDownloadsInputMode>(
    source.automaticDownloads?.mode ?? SourceAutomaticDownloadsInputMode.defaults,
  );
  const [everySeconds, setEverySeconds] = useState(source.automaticDownloads?.everySeconds || 6 * 3600);
  const [maxPerCheck, setMaxPerCheck] = useState(source.automaticDownloads?.maxPerCheck ?? 10);
  const [editingCustomSchedule, setEditingCustomSchedule] = useState(
    downloadSchedulePreset(source.automaticDownloads?.everySeconds || 6 * 3600) === "custom",
  );
  const [saveError, setSaveError] = useState<string>();
  const saveButton = useRef<ComponentRef<typeof Action>>(null);
  const automaticDownloads = source.automaticDownloads;
  if (!automaticDownloads) return null;
  const interval = downloadIntervalParts(everySeconds);

  return (
    <div className="flex flex-col gap-3">
      <div>
        <p className="text-sm font-medium">Automatic downloads</p>
        <p className="text-muted-foreground mt-1 text-sm">{automaticDownloads.summary}</p>
        {automaticDownloads.nextCheckAt && (
          <p className="text-muted-foreground mt-1 text-xs">
            Next automatic check {formatRelative(automaticDownloads.nextCheckAt)}
          </p>
        )}
      </div>
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
      {mode === "custom" && (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div className="flex flex-col gap-1.5">
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
          </div>
          <div className="flex flex-col gap-1.5">
            <span className="text-sm">Add up to</span>
            <NumberField
              accessibilityLabel="Clips per source check"
              amount={maxPerCheck}
              onAmount={(amount) => setMaxPerCheck(Math.min(1000, Math.max(amount, 1)))}
            />
          </div>
          {editingCustomSchedule && (
            <div className="flex flex-col gap-1.5 sm:col-span-2">
              <span className="text-sm">Custom schedule</span>
              <div className="flex gap-2">
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
              </div>
            </div>
          )}
        </div>
      )}
      <Action
        disabled={actions.updating}
        ref={saveButton}
        style={{ alignSelf: "flex-start" }}
        variant="outline"
        onPress={() => {
          setSaveError(undefined);
          actions.updateSource(
            source.id,
            {
              enabled: source.enabled,
              automaticDownloads:
                mode === "custom" ? { mode: "custom", everySeconds, maxPerCheck } : { mode },
            },
            {
              onSuccess: () => toast.success("Automatic downloads updated"),
              onError: (error) =>
                setSaveError(error.detail ?? "Automatic downloads couldn’t be saved. Try again."),
              onSettled: () => requestAnimationFrame(() => saveButton.current?.focus()),
            },
          );
        }}
      >
        {actions.updatingSourceID === source.id ? "Saving…" : "Save automatic downloads"}
      </Action>
      {saveError && (
        <p role="alert" className="bg-destructive/10 text-destructive rounded-md px-3 py-2 text-sm">
          {saveError}
        </p>
      )}
    </div>
  );
};

export {
  AutomaticDownloadsForm,
  downloadIntervalMaximum,
  downloadIntervalParts,
  downloadIntervalSeconds,
  downloadSchedulePreset,
};
