import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FillerStorageStatusDTO } from "@loomarr/api/models/fillerStorageStatusDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { formatBytes } from "@loomarr/core/format";
import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { type SettingsBlock, SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";
import { ClipDetailsSettings } from "../clip-details-settings";
import { type FillerSettingsSection, SETTINGS_SECTIONS } from "../filler-settings-section";

const downloadIntervalSeconds = (value: string): number => {
  if (value === "0") return 0;
  const factors: Record<string, number> = { d: 86400, h: 3600, m: 60, s: 1 };
  let seconds = 0;
  let matched = false;
  for (const match of value.matchAll(/(-?\d+(?:\.\d+)?)(d|h|m|s)/g)) {
    matched = true;
    seconds += Number(match[1]) * (factors[match[2] ?? ""] ?? 0);
  }
  return matched ? seconds : Number.NaN;
};

const downloadIntervalLabel = (value: string): string => {
  const seconds = downloadIntervalSeconds(value);
  if (seconds === 86400) return "Daily";
  if (seconds === 7 * 86400) return "Weekly";
  if (seconds > 0 && seconds % 86400 === 0) {
    const days = seconds / 86400;
    return `Every ${days} ${days === 1 ? "day" : "days"}`;
  }
  if (seconds > 0 && seconds % 3600 === 0) {
    const hours = seconds / 3600;
    return `Every ${hours} ${hours === 1 ? "hour" : "hours"}`;
  }
  if (seconds > 0 && seconds % 60 === 0) {
    const minutes = seconds / 60;
    return `Every ${minutes} ${minutes === 1 ? "minute" : "minutes"}`;
  }
  return `Every ${value}`;
};

const storagePauseMessage = (pausedBy?: FillerStorageStatusDTO["pausedBy"]): string | undefined => {
  switch (pausedBy) {
    case "library_limit":
      return "New filler is paused because the library reached its allowance. Remove clips or increase it.";
    case "host_reserve":
      return "New filler is paused because this drive is running low. Free some space to continue.";
    case "capacity_unavailable":
    case "estimate_unknown":
      return "Loomarr cannot safely check this folder's available space. Check the drive or choose another folder.";
    default:
      return undefined;
  }
};

const StorageSummary = ({ storage }: { storage?: FillerStorageStatusDTO }) => {
  if (!storage) {
    return (
      <p aria-live="polite" className="rounded-lg bg-muted/50 px-4 py-3 text-muted-foreground text-sm">
        Checking this drive…
      </p>
    );
  }
  const pauseMessage = storagePauseMessage(storage.pausedBy);
  return (
    <div
      className={
        pauseMessage
          ? "rounded-lg border border-caution/40 bg-caution/5 px-4 py-3 text-sm"
          : "rounded-lg bg-muted/50 px-4 py-3 text-sm"
      }
    >
      <p className="font-medium">
        {storage.automatic ? "Automatic" : "Custom"} {formatBytes(storage.softBudgetBytes)} allowance
      </p>
      <p className="mt-1 text-muted-foreground">
        {formatBytes(storage.managedBytes)} used · {formatBytes(storage.availableBytes)} available to Loomarr.
        Loomarr keeps at least {formatBytes(storage.hardReserveBytes)} free for this device.
      </p>
      {pauseMessage ? <p className="mt-2 text-caution-foreground">{pauseMessage}</p> : null}
    </div>
  );
};

const FillerSettingsTaskSwitcher = ({ section }: { section: FillerSettingsSection }) => {
  const navigate = useNavigate();
  return (
    <div className="flex flex-col gap-3 rounded-lg border border-border bg-card p-4 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <Link
          to="/filler/settings"
          className="inline-flex items-center gap-1.5 text-muted-foreground text-sm hover:text-foreground"
        >
          <ArrowLeft className="size-3.5" aria-hidden />
          All filler settings
        </Link>
        <p className="mt-2 text-muted-foreground text-xs">Jump straight to another task.</p>
      </div>
      <Select
        value={section}
        onValueChange={(next) =>
          void navigate({
            to: "/filler/settings/$section",
            params: { section: next as FillerSettingsSection },
          })
        }
      >
        <SelectTrigger className="w-full sm:w-64" aria-label="Filler settings task">
          <SelectValue />
        </SelectTrigger>
        <SelectContent align="end">
          {SETTINGS_SECTIONS.map((item) => (
            <SelectItem key={item.id} value={item.id}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
};

const FillerSettingsIndex = () => (
  <div className="space-y-8">
    <div>
      <Link
        to="/filler/manage"
        className="inline-flex items-center gap-1.5 text-muted-foreground text-sm hover:text-foreground"
      >
        <ArrowLeft className="size-3.5" aria-hidden />
        Manage
      </Link>
      <h2 className="mt-3 font-semibold text-xl">Filler settings</h2>
      <p className="mt-1 text-muted-foreground text-sm">
        Change how Loomarr finds, prepares, stores, and plays short clips.
      </p>
    </div>

    {(
      [
        {
          id: "everyday",
          label: "Everyday choices",
          description: "The settings most people are likely to change.",
        },
        {
          id: "advanced",
          label: "Advanced tuning",
          description: "Processing capacity and tools for unusual installations.",
        },
      ] as const
    ).map((group) => (
      <section key={group.id} aria-labelledby={`filler-settings-${group.id}`}>
        <div className="mb-3">
          <h3 id={`filler-settings-${group.id}`} className="font-medium text-base">
            {group.label}
          </h3>
          <p className="mt-0.5 text-muted-foreground text-sm">{group.description}</p>
        </div>
        <div className="grid gap-2 md:grid-cols-2">
          {SETTINGS_SECTIONS.filter((item) => item.group === group.id).map((item) => (
            <Link
              key={item.id}
              to="/filler/settings/$section"
              params={{ section: item.id }}
              className="group flex min-w-0 items-start gap-3 rounded-lg border border-border bg-card px-4 py-3 transition-colors hover:border-signal/40 hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <span className="min-w-0 flex-1">
                <span className="block font-medium text-sm">{item.label}</span>
                <span className="mt-0.5 block text-muted-foreground text-xs leading-relaxed">
                  {item.description}
                </span>
              </span>
              <ArrowRight
                className="mt-0.5 size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:text-foreground"
                aria-hidden
              />
            </Link>
          ))}
        </div>
      </section>
    ))}
  </div>
);

const FillerSettings = ({ section = "downloads" }: { section?: FillerSettingsSection }) => {
  const entries = useSettingsEntries();
  const sourcesQuery = fillerApi.useListFillerSources({ query: { enabled: section === "downloads" } });
  const readinessQuery = fillerApi.useFillerReadiness({ query: { enabled: section === "storage" } });
  const sources = unwrap(sourcesQuery.data, (body) => body.sources) ?? [];
  const storage = unwrap(readinessQuery.data, (body) => body.storage);
  const blocks: (SettingsBlock & { section: FillerSettingsSection })[] = [
    {
      section: "folders",
      group: "filler",
      title: "Clip folders",
      description: "Where clips are stored and how files dropped onto this machine enter the catalog.",
      keys: ["filler.dir", "filler.watch_dir", "filler.source.folder.enabled", "filler.sync_every"],
    },
    {
      section: "downloads",
      group: "filler",
      title: "Automatic downloads",
      description: "Choose how often Loomarr looks for clips and how many each source may add.",
      keys: ["filler.fetch.every", "filler.fetch.max_per_run"],
      footer: ({ liveValue }) => {
        const every = liveValue("filler.fetch.every") || "6h";
        const defaultMax = Math.max(1, Number(liveValue("filler.fetch.max_per_run")) || 10);
        const activeSources = sources.filter(
          (source) =>
            !source.group &&
            source.configured &&
            source.effectiveEnabled &&
            source.fetchable &&
            (source.kind === "archive" || source.kind === "youtube") &&
            (source.automaticDownloads?.everySeconds ?? 0) > 0,
        );
        const fullCheckMax = activeSources.reduce(
          (total, source) =>
            total +
            (source.automaticDownloads?.mode === "defaults"
              ? defaultMax
              : (source.automaticDownloads?.maxPerCheck ?? defaultMax)),
          0,
        );
        const sourceCount = activeSources.length;
        const cadence =
          downloadIntervalSeconds(every) === 0
            ? "Automatic downloads are off for sources using these defaults."
            : `${downloadIntervalLabel(every)}, each source using these defaults can add up to ${defaultMax} ${defaultMax === 1 ? "clip" : "clips"}.`;
        const round =
          sourceCount === 0
            ? "No enabled sources are currently downloading automatically."
            : `Across ${sourceCount} enabled ${sourceCount === 1 ? "source" : "sources"}, one full check can add up to ${fullCheckMax} ${fullCheckMax === 1 ? "clip" : "clips"}.`;
        return (
          <p className="rounded-lg bg-muted/50 px-4 py-3 text-muted-foreground text-sm">
            {cadence} {round}{" "}
            <Link to="/filler/settings/$section" params={{ section: "storage" }} className="underline">
              Storage limits
            </Link>
          </p>
        );
      },
    },
    {
      section: "storage",
      group: "filler",
      title: "Storage",
      description: "Loomarr chooses a safe allowance automatically. Set your own only if you need to.",
      keys: ["filler.storage.library_budget_gb", "filler.fetch.max_catalog_clips"],
      footer: <StorageSummary storage={storage} />,
    },
    {
      section: "details",
      group: "filler",
      title: "Clip details",
      description: "Help Loomarr identify when and where a clip was made.",
      keys: ["filler.research.enabled"],
      footer: ({ liveValue, setEdit }) => (
        <ClipDetailsSettings entries={entries} liveValue={liveValue} setEdit={setEdit} />
      ),
    },
    {
      section: "incoming",
      group: "filler",
      title: "Incoming history",
      description: "Choose how long newly ready clips stay visible in Incoming.",
      keys: ["filler.incoming.ready_window"],
      initialAdvanced: true,
    },
    {
      section: "breaks",
      group: "filler",
      title: "Break assembly",
      description:
        "Choose the usual break length and number of clips. Each channel can use its own length and frequency.",
      keys: ["filler.break_duration", "filler.pod_max"],
    },
    {
      section: "review",
      group: "filler",
      title: "Clip review",
      description: "Choose which background checks may identify, split, or set aside incoming clips.",
      keys: [
        "filler.transcribe.enabled",
        "filler.vision.enabled",
        "filler.autosplit.enabled",
        "filler.autosplit.min_confidence",
        "filler.autosplit.max_duration",
      ],
    },
    {
      section: "playback",
      group: "filler",
      title: "Clip eligibility and sound",
      initialAdvanced: true,
      keys: [
        "filler.cooldown_seconds",
        "filler.min_quality",
        "filler.weight",
        "filler.min_duration",
        "filler.split.review_window",
        "filler.min_clip_duration",
        "filler.max_clip_duration",
        "filler.target_lufs",
      ],
    },
    {
      section: "limits",
      group: "filler",
      title: "Processing limits",
      description: "Choose how much background work Loomarr can do at once.",
      initialAdvanced: true,
      keys: [
        "filler.pipeline.max_clips",
        "filler.transcode.max_per_run",
        "filler.pipeline.max_whisper",
        "filler.pipeline.max_vision",
        "filler.pipeline.max_split_vision",
        "filler.pipeline.max_splits",
      ],
    },
    {
      section: "tools",
      group: "filler",
      title: "Processing tools",
      initialAdvanced: true,
      description: "Executable and model paths for unusual source installs. The container supplies these.",
      keys: [
        "ingest.ytdlp_path",
        "ingest.ffmpeg_path",
        "ingest.timeout",
        "ingest.whisper_path",
        "ingest.whisper_model",
      ],
    },
  ];
  return (
    <SettingsPage
      key={section}
      embedded
      title="Filler settings"
      description="Change the defaults here. Individual sources and channels can use their own settings."
      entries={entries}
      blocks={blocks.filter((block) => block.section === section)}
    >
      <FillerSettingsTaskSwitcher section={section} />
    </SettingsPage>
  );
};

export { FillerSettings, FillerSettingsIndex };
