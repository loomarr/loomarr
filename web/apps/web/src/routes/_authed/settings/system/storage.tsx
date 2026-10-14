import { createFileRoute } from "@tanstack/react-router";
import { SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";

const StorageSettings = () => (
  <SettingsPage
    title="Storage"
    description="Where artwork lives and the disk and network limits around it. Database backups do not include these files."
    entries={useSettingsEntries()}
    blocks={[
      {
        group: "images",
        title: "Images",
        keys: [
          "images.dir",
          "images.remote_fetch_enabled",
          "images.max_upload_bytes",
          "images.cache_budget_mb",
        ],
      },
    ]}
    footer={
      <aside className="rounded-lg border border-caution/40 bg-caution/5 p-4 text-sm">
        <h2 className="font-medium">Back up the image volume separately</h2>
        <p className="mt-1 text-muted-foreground">
          Downloaded artwork can be rebuilt, but uploaded channel images cannot. Loomarr's database backup
          does not copy this directory.
        </p>
      </aside>
    }
  />
);

const Route = createFileRoute("/_authed/settings/system/storage")({
  component: StorageSettings,
});

export { Route };
