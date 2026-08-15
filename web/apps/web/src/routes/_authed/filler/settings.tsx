import { createFileRoute, Link } from "@tanstack/react-router";
import { NavTabs } from "@/components/ui";
import { SettingsEditsProvider, SettingsPage, SettingsSaveBarHost, useSettingsEntries } from "@/settings";

const FillerOperations = () => (
  <SettingsPage
    title="Filler settings"
    description="Where clips arrive, what Loomarr may automate, and the limits that keep background processing bounded. Channel break defaults remain under Settings → Defaults."
    entries={useSettingsEntries()}
    blocks={[
      {
        group: "filler",
        title: "Library and sources",
        keys: [
          "filler.dir",
          "filler.watch_dir",
          "filler.sync_every",
          "filler.source.folder.enabled",
          "filler.fetch.every",
          "filler.fetch.max_per_run",
          "filler.fetch.max_catalog_clips",
          "filler.fetch.max_disk_gb",
        ],
      },
      {
        group: "filler",
        title: "Automation",
        keys: [
          "filler.ai_tagging",
          "filler.autofile.enabled",
          "filler.autofile.min_confidence",
          "filler.autofile.normalize_loudness",
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
          "filler.language_provider",
        ],
      },
      {
        group: "filler",
        title: "Pipeline limits",
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
