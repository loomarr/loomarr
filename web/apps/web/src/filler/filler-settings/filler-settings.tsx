import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FillerStorageStatusDTO } from "@loomarr/api/models/fillerStorageStatusDTO";
import { SettingKey } from "@loomarr/api/models/settingKey";
import type { SettingPreset } from "@loomarr/api/models/settingPreset";
import { unwrap } from "@loomarr/api/unwrap";
import { formatBytes, goDurationMinutes } from "@loomarr/core/format";
import { Action, Icon, icons, Stack, Surface, Text } from "@loomarr/design-system";
import { Link, useNavigate } from "@tanstack/react-router";
import { BrowserSelectControl } from "@/client-adapters";
import { type SettingsBlock, SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";
import { ClipDetailsSettings } from "../clip-details-settings";
import { type FillerSettingsSection, SETTINGS_SECTIONS } from "../filler-settings-section";
import { useFillerProviders } from "../use-filler-providers";

// The interval named as the field's own choices name it: the registry preset's label when the value
// is one of them, otherwise the custom interval spelled out.
const downloadIntervalLabel = (value: string, presets: readonly SettingPreset[]): string => {
  const minutes = goDurationMinutes(value);
  const preset = presets.find((p) => goDurationMinutes(p.value) === minutes);
  if (minutes === undefined) return `Every ${value}`;
  if (preset) return preset.label;
  const seconds = minutes * 60;
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
      return "Loomarr cannot safely check this folder's available space. Check the drive or choose another folder.";
    case "estimate_unknown":
      return "A download could not be sized safely and was stopped. The folder is fine; Loomarr will try again.";
    default:
      return undefined;
  }
};

const StorageSummary = ({ storage }: { storage?: FillerStorageStatusDTO }) => {
  if (!storage) {
    return (
      <p aria-live="polite" className="bg-muted/50 text-muted-foreground rounded-lg px-4 py-3 text-sm">
        Checking this drive…
      </p>
    );
  }
  const pauseMessage = storagePauseMessage(storage.pausedBy);
  // The server owns the warning threshold (storage state); a nearly full or full allowance offers
  // the way out (#2737), which is the allowance field just above.
  const allowanceFull = storage.state === "approaching" || storage.pausedBy === "library_limit";
  return (
    <div
      role="status"
      className={
        pauseMessage || allowanceFull
          ? "border-caution/40 bg-caution/5 rounded-lg border px-4 py-3 text-sm"
          : "bg-muted/50 rounded-lg px-4 py-3 text-sm"
      }
    >
      <p className="font-medium">
        {storage.automatic ? "Automatic" : "Custom"} {formatBytes(storage.softBudgetBytes)} allowance
      </p>
      <Text as="p" style={{ marginTop: 4 }} textRole="body" tone={allowanceFull ? "warning" : "muted"}>
        {formatBytes(storage.managedBytes)} used · {formatBytes(storage.availableBytes)} available to Loomarr.
        Loomarr keeps at least {formatBytes(storage.hardReserveBytes)} free for this device.
      </Text>
      {pauseMessage ? <p className="text-caution-foreground mt-2">{pauseMessage}</p> : null}
      {allowanceFull ? (
        <Surface alignItems="flex-start" backgroundColor="transparent" borderWidth={0} marginTop={12}>
          <Action
            variant="outline"
            onPress={() =>
              document.getElementById(`setting-${SettingKey.FillerStorageLibraryBudgetGb}`)?.focus()
            }
          >
            Raise allowance
          </Action>
        </Surface>
      ) : null}
    </div>
  );
};

const FillerSettingsTaskSwitcher = ({ section }: { section: FillerSettingsSection }) => {
  const navigate = useNavigate();
  return (
    <div className="border-border bg-card flex flex-col gap-3 rounded-lg border p-4 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <Link
          to="/filler/settings"
          className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1.5 text-sm"
        >
          <Icon decorative glyph={icons.goBack} size="compact" tone="inherit" />
          All filler settings
        </Link>
        <p className="text-muted-foreground mt-2 text-xs">Jump straight to another task.</p>
      </div>
      <div className="w-full sm:w-64">
        <BrowserSelectControl
          appearance="field"
          label="Filler settings task"
          labelHidden
          onValueChange={(next) =>
            void navigate({ to: "/filler/settings/$section", params: { section: next } })
          }
          options={SETTINGS_SECTIONS.map((item) => ({ label: item.label, value: item.id }))}
          value={section}
        />
      </div>
    </div>
  );
};

const FillerSettingsIndex = () => (
  <div className="space-y-8">
    <div>
      <Link
        to="/filler/manage"
        className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1.5 text-sm"
      >
        <Icon decorative glyph={icons.goBack} size="compact" tone="inherit" />
        Manage
      </Link>
      <h2 className="mt-3 text-xl font-semibold">Filler settings</h2>
      <p className="text-muted-foreground mt-1 text-sm">
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
          <h3 id={`filler-settings-${group.id}`} className="text-base font-medium">
            {group.label}
          </h3>
          <p className="text-muted-foreground mt-0.5 text-sm">{group.description}</p>
        </div>
        <div className="grid gap-2 md:grid-cols-2">
          {SETTINGS_SECTIONS.filter((item) => item.group === group.id).map((item) => (
            <Link
              key={item.id}
              to="/filler/settings/$section"
              params={{ section: item.id }}
              className="group border-border bg-card hover:border-signal/40 hover:bg-muted/40 focus-visible:ring-ring flex min-w-0 items-start gap-3 rounded-lg border px-4 py-3 transition-colors focus-visible:ring-2 focus-visible:outline-none"
            >
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-medium">{item.label}</span>
                <span className="text-muted-foreground mt-0.5 block text-xs leading-relaxed">
                  {item.description}
                </span>
              </span>
              <Stack
                className="text-muted-foreground group-hover:text-foreground mt-0.5 transition-colors"
                direction="row"
                render="span"
              >
                <Icon decorative glyph={icons.goForward} size="compact" tone="inherit" />
              </Stack>
            </Link>
          ))}
        </div>
      </section>
    ))}
  </div>
);

const FillerSettings = ({ section = "downloads" }: { section?: FillerSettingsSection }) => {
  const entries = useSettingsEntries();
  const providers = useFillerProviders();
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
      keys: [
        SettingKey.FillerDir,
        SettingKey.FillerWatchDir,
        SettingKey.FillerSourceFolderEnabled,
        SettingKey.FillerSyncEvery,
      ],
    },
    {
      section: "downloads",
      group: "filler",
      title: "Automatic downloads",
      description: "Choose how often Loomarr looks for clips and how many each source may add.",
      keys: [SettingKey.FillerFetchEvery, SettingKey.FillerFetchMaxPerRun, SettingKey.FillerRegionFallback],
      footer: ({ liveValue }) => {
        const every = liveValue(SettingKey.FillerFetchEvery) || "6h";
        const defaultMax = Math.max(1, Number(liveValue(SettingKey.FillerFetchMaxPerRun)) || 10);
        const activeSources = sources.filter(
          (source) =>
            !source.group &&
            source.configured &&
            source.effectiveEnabled &&
            source.fetchable &&
            providers.find(source.kind)?.fetched &&
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
        const presets = entries.find((entry) => entry.key === SettingKey.FillerFetchEvery)?.presets ?? [];
        const cadence =
          goDurationMinutes(every) === 0
            ? "Automatic downloads are off for sources using these defaults."
            : `${downloadIntervalLabel(every, presets)}, each source using these defaults can add up to ${defaultMax} ${defaultMax === 1 ? "clip" : "clips"}.`;
        const round =
          sourceCount === 0
            ? "No enabled sources are currently downloading automatically."
            : `Across ${sourceCount} enabled ${sourceCount === 1 ? "source" : "sources"}, one full check can add up to ${fullCheckMax} ${fullCheckMax === 1 ? "clip" : "clips"}.`;
        return (
          <p className="bg-muted/50 text-muted-foreground rounded-lg px-4 py-3 text-sm">
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
      keys: [SettingKey.FillerStorageLibraryBudgetGb, SettingKey.FillerFetchMaxCatalogClips],
      footer: <StorageSummary storage={storage} />,
    },
    {
      section: "details",
      group: "filler",
      title: "Clip details",
      description: "Help Loomarr identify when and where a clip was made.",
      keys: [SettingKey.FillerResearchEnabled],
      footer: ({ liveValue, setEdit }) => (
        <ClipDetailsSettings entries={entries} liveValue={liveValue} setEdit={setEdit} />
      ),
    },
    {
      section: "incoming",
      group: "filler",
      title: "Incoming history",
      description: "Choose how long newly ready clips stay visible in Incoming.",
      keys: [SettingKey.FillerIncomingReadyWindow],
      initialAdvanced: true,
    },
    {
      section: "breaks",
      group: "filler",
      title: "Break assembly",
      description:
        "Choose the usual break length and number of clips. Each channel can use its own length and frequency.",
      keys: [SettingKey.FillerBreakDuration, SettingKey.FillerPodMax],
    },
    {
      section: "review",
      group: "filler",
      title: "Clip review",
      description: "Choose which background checks may identify, split, or set aside incoming clips.",
      keys: [
        SettingKey.FillerTranscribeEnabled,
        SettingKey.FillerVisionEnabled,
        SettingKey.FillerAutosplitEnabled,
        SettingKey.FillerAutosplitMinConfidence,
        SettingKey.FillerAutosplitMaxDuration,
      ],
    },
    {
      section: "playback",
      group: "filler",
      title: "Clip eligibility and sound",
      initialAdvanced: true,
      keys: [
        SettingKey.FillerCooldownSeconds,
        SettingKey.FillerMinQuality,
        SettingKey.FillerWeight,
        SettingKey.FillerMinDuration,
        SettingKey.FillerSplitReviewWindow,
        SettingKey.FillerMinClipDuration,
        SettingKey.FillerMaxClipDuration,
        SettingKey.FillerTargetLufs,
      ],
    },
    {
      section: "limits",
      group: "filler",
      title: "Processing limits",
      description: "Choose how much background work Loomarr can do at once.",
      initialAdvanced: true,
      keys: [
        SettingKey.FillerPipelineMaxClips,
        SettingKey.FillerTranscodeMaxPerRun,
        SettingKey.FillerPipelineMaxWhisper,
        SettingKey.FillerPipelineMaxVision,
        SettingKey.FillerPipelineMaxSplitVision,
        SettingKey.FillerPipelineMaxSplits,
        SettingKey.FillerDefectsJudgeLookCents,
        SettingKey.FillerDefectsJudgeDailyCents,
      ],
    },
    {
      section: "tools",
      group: "filler",
      title: "Processing tools",
      initialAdvanced: true,
      description: "Executable and model paths for unusual source installs. The container supplies these.",
      keys: [
        SettingKey.IngestYtdlpPath,
        SettingKey.IngestFFmpegPath,
        SettingKey.IngestTimeout,
        SettingKey.IngestWhisperPath,
        SettingKey.IngestWhisperModel,
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
