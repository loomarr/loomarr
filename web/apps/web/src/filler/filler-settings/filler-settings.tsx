import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import { unwrap } from "@loomarr/api/unwrap";
import { Link, useSearch } from "@tanstack/react-router";
import { CollapsibleSection } from "@/components/loomarr/feedback/collapsible-section";
import { type SettingsBlock, SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";
import { type FillerSettingsSection, SETTINGS_SECTIONS } from "../filler-settings-search";

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

const FillerSettings = () => {
  const search = useSearch({ strict: false }) as { section?: FillerSettingsSection };
  const section = search.section ?? "downloads";
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
            <Link to="/filler/settings" search={{ section: "storage" }} className="underline">
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
      footer={
        <CollapsibleSection
          title="More settings"
          description="Folders, storage, breaks, and advanced controls."
        >
          <nav aria-label="Filler settings tasks" className="grid gap-3 sm:grid-cols-2">
            {SETTINGS_SECTIONS.filter((item) => item.id !== section).map((item) => (
              <Link
                key={item.id}
                to="/filler/settings"
                search={{ section: item.id }}
                className="text-sm underline underline-offset-4"
              >
                {item.label}
              </Link>
            ))}
          </nav>
        </CollapsibleSection>
      }
    />
  );
};

export { FillerSettings };
