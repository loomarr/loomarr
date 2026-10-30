import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";
import type { SourceAutomaticDownloadsInputMode } from "@loomarr/api/models/sourceAutomaticDownloadsInputMode";
import { formatRelative } from "@loomarr/core/format";
import { useRef, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
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

interface AutomaticDownloadsFormProps {
  source: FillerSourceDTO;
  actions: Pick<SourceActions, "updateSource" | "updating" | "updatingSourceID">;
}

// Per-source "Automatic downloads": follow the defaults, use a schedule of its own, or never. The
// draft is seeded from the source once, when the form mounts — remount per source (`key`) — so a
// background refetch cannot overwrite what the operator is mid-way through typing.
const AutomaticDownloadsForm = ({ source, actions }: AutomaticDownloadsFormProps) => {
  const [mode, setMode] = useState<SourceAutomaticDownloadsInputMode>(
    source.automaticDownloads?.mode ?? "defaults",
  );
  const [everySeconds, setEverySeconds] = useState(source.automaticDownloads?.everySeconds || 6 * 3600);
  const [maxPerCheck, setMaxPerCheck] = useState(source.automaticDownloads?.maxPerCheck ?? 10);
  const [editingCustomSchedule, setEditingCustomSchedule] = useState(
    downloadSchedulePreset(source.automaticDownloads?.everySeconds || 6 * 3600) === "custom",
  );
  const [saveError, setSaveError] = useState<string>();
  const saveButton = useRef<HTMLButtonElement>(null);
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
      <Select
        value={mode}
        onValueChange={(value) => {
          setMode(value as SourceAutomaticDownloadsInputMode);
          if (value === "custom") {
            setEditingCustomSchedule(downloadSchedulePreset(everySeconds) === "custom");
          }
        }}
      >
        <SelectTrigger aria-label="Automatic downloads for this source">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="defaults">Use automatic-download defaults</SelectItem>
          <SelectItem value="custom">Use a different schedule</SelectItem>
          <SelectItem value="never">Never download automatically</SelectItem>
        </SelectContent>
      </Select>
      {mode === "custom" && (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div className="flex flex-col gap-1.5">
            <span className="text-sm">Look for new clips</span>
            <Select
              value={editingCustomSchedule ? "custom" : downloadSchedulePreset(everySeconds)}
              onValueChange={(value) => {
                const preset = value as DownloadSchedulePreset;
                setEditingCustomSchedule(preset === "custom");
                if (preset !== "custom") {
                  setEverySeconds(downloadScheduleSeconds[preset]);
                }
              }}
            >
              <SelectTrigger aria-label="Source check schedule">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="6h">Every 6 hours</SelectItem>
                <SelectItem value="12h">Every 12 hours</SelectItem>
                <SelectItem value="daily">Daily</SelectItem>
                <SelectItem value="weekly">Weekly</SelectItem>
                <SelectItem value="custom">Custom</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-1.5">
            <span className="text-sm">Add up to</span>
            <Input
              type="number"
              min={1}
              max={1000}
              aria-label="Clips per source check"
              value={maxPerCheck}
              onChange={(event) => setMaxPerCheck(Math.min(1000, Math.max(Number(event.target.value), 1)))}
            />
          </div>
          {editingCustomSchedule && (
            <div className="flex flex-col gap-1.5 sm:col-span-2">
              <span className="text-sm">Custom schedule</span>
              <div className="flex gap-2">
                <Input
                  type="number"
                  min={1}
                  max={downloadIntervalMaximum(interval.unit)}
                  step="any"
                  aria-label="Source check interval"
                  value={interval.amount}
                  onChange={(event) => {
                    const maximum = downloadIntervalMaximum(interval.unit);
                    setEverySeconds(
                      downloadIntervalSeconds(Math.min(Number(event.target.value), maximum), interval.unit),
                    );
                  }}
                />
                <Select
                  value={interval.unit}
                  onValueChange={(value) => {
                    setEverySeconds(
                      downloadIntervalSeconds(
                        Math.min(interval.amount, downloadIntervalMaximum(value as DownloadIntervalUnit)),
                        value as DownloadIntervalUnit,
                      ),
                    );
                  }}
                >
                  <SelectTrigger aria-label="Source check interval unit" className="w-28 shrink-0">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="minutes">minutes</SelectItem>
                    <SelectItem value="hours">hours</SelectItem>
                    <SelectItem value="days">days</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}
        </div>
      )}
      <Button
        ref={saveButton}
        type="button"
        variant="outline"
        className="self-start"
        disabled={actions.updating}
        onClick={() => {
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
      </Button>
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
