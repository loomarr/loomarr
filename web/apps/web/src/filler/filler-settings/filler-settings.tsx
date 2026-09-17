import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import { unwrap } from "@loomarr/api/unwrap";
import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { type SettingsBlock, SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";
import { type FillerSettingsSection, SETTINGS_SECTIONS } from "../filler-settings-section";

const settingValue = (entries: SettingEntry[], key: string): string =>
  entries.find((entry) => entry.key === key)?.value ?? "";

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

// The language stage already skips with these same configuration facts. Mirror them at the
// decision point so `en` cannot look active while every clip is actually passing unchecked.
const languageUnavailableReason = (entries: SettingEntry[]): string | undefined => {
  const provider = settingValue(entries, "filler.language_provider") || "whisper";
  if (provider === "hosted") {
    if (settingValue(entries, "llm.url") === "") {
      return "Language filtering is off because the hosted AI service address is not configured. Set it under Settings → AI.";
    }
    if (settingValue(entries, "llm.model") === "") {
      return "Language filtering is off because the hosted language model is not configured. Set it under Settings → AI.";
    }
  } else {
    if (settingValue(entries, "ingest.whisper_path") === "") {
      return "Language filtering is off because the local language engine is not configured. Set the whisper executable under Processing tools.";
    }
    if (settingValue(entries, "filler.language_model") === "") {
      return "Language filtering is off because no multilingual detection model is configured. Add one under Settings → AI.";
    }
  }
  if (settingValue(entries, "playout.ffmpeg_path") === "") {
    return "Language filtering is off because audio extraction is not configured. Set the ffmpeg executable under System → Playback.";
  }
  return undefined;
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
  const languageReason = languageUnavailableReason(entries);
  const sourcesQuery = fillerApi.useListFillerSources({ query: { enabled: section === "downloads" } });
  const sources = unwrap(sourcesQuery.data, (body) => body.sources) ?? [];
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
      title: "Storage limits",
      description: "Stop automatic downloads before the filler library takes too much space.",
      keys: ["filler.fetch.max_catalog_clips", "filler.fetch.max_disk_gb"],
      initialAdvanced: true,
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
        "Default break length and clip density. A channel can override its length; frequency stays under Settings → Defaults.",
      keys: ["filler.break_duration", "filler.pod_max"],
    },
    {
      section: "review",
      group: "filler",
      title: "Clip review",
      description: "Choose which background checks may identify, split, or set aside incoming clips.",
      keys: [
        "filler.ai_tagging",
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
      disabledReasons: languageReason ? { "filler.language": languageReason } : undefined,
      keys: [
        "filler.cooldown_seconds",
        "filler.min_quality",
        "filler.weight",
        "filler.min_duration",
        "filler.split.review_window",
        "filler.min_clip_duration",
        "filler.max_clip_duration",
        "filler.target_lufs",
        "filler.language",
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
