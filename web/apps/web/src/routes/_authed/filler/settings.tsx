import { createFileRoute, Link } from "@tanstack/react-router";
import { NavTabs } from "@/components/ui";
import { SettingsEditsProvider, SettingsPage, SettingsSaveBarHost, useSettingsEntries } from "@/settings";

const FillerOperations = () => (
  <SettingsPage
    title="Filler settings"
    description="Where clips arrive, how breaks are assembled, and the limits that keep background processing bounded. Per-channel frequency and clip matching live on each channel's Filler page."
    entries={useSettingsEntries()}
    blocks={[
      {
        group: "filler",
        title: "Clip folders",
        description: "Where clips are stored and how files dropped onto this machine enter the catalog.",
        keys: ["filler.dir", "filler.watch_dir", "filler.source.folder.enabled", "filler.sync_every"],
      },
      {
        group: "filler",
        title: "Automatic downloads",
        description: "How often enabled sources are checked. Safety limits stay available under Advanced.",
        keys: [
          "filler.fetch.every",
          "filler.fetch.max_per_run",
          "filler.fetch.max_catalog_clips",
          "filler.fetch.max_disk_gb",
        ],
      },
      {
        group: "filler",
        title: "Break assembly",
        description:
          "Global limits for every channel. The inherited break frequency stays under Settings → Defaults.",
        keys: ["filler.pod_max"],
      },
      {
        group: "filler",
        title: "Clip review",
        description: "Choose which background checks may identify, split, or set aside incoming clips.",
        keys: [
          "filler.ai_tagging",
          "filler.transcribe.enabled",
          "filler.vision.enabled",
          "filler.reindex.enabled",
          "filler.autosplit.enabled",
          "filler.autosplit.min_confidence",
          "filler.autosplit.max_duration",
          "filler.reject.unidentified",
        ],
      },
      {
        group: "filler",
        title: "Clip eligibility and sound",
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
        group: "filler",
        title: "Pipeline limits",
        description: "Per-pass budgets that keep background work from taking over the machine.",
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
        group: "filler",
        title: "Processing tools",
        description: "Executable and model paths for unusual source installs. The container supplies these.",
        keys: [
          "ingest.ytdlp_path",
          "ingest.ffmpeg_path",
          "ingest.timeout",
          "ingest.whisper_path",
          "ingest.whisper_model",
        ],
      },
    ]}
  />
);

const FillerSettingsScreen = () => (
  <SettingsEditsProvider>
    <div className="flex h-full min-h-0 flex-col">
      <NavTabs
        label="Filler sections"
        linkComponent={Link}
        className="bg-background px-6 pt-2"
        activeId="settings"
        tabs={[
          { id: "catalog", label: "Catalog", to: "/filler" },
          { id: "incoming", label: "Incoming", to: "/filler/incoming" },
          { id: "sources", label: "Sources", to: "/filler/sources" },
          { id: "settings", label: "Settings", to: "/filler/settings" },
        ]}
      />
      <div className="min-h-0 flex-1 overflow-hidden">
        <FillerOperations />
      </div>
      <SettingsSaveBarHost />
    </div>
  </SettingsEditsProvider>
);

const Route = createFileRoute("/_authed/filler/settings")({
  component: FillerSettingsScreen,
});

export { Route };
