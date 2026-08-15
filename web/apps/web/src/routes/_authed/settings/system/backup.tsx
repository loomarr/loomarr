import { systemApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useCallback, useState } from "react";
import { BackupPanel, ErrorState } from "@/components/loomarr";
import { SettingsPage, useSettingsEntries } from "@/settings";

// Settings → System → Backup (§16, V12) — the backups on disk.
//
// ⚠ The download is a plain same-origin NAVIGATION, not a fetch. The session is a cookie,
// so the browser carries it; and streaming straight to the browser's downloader means a
// multi-hundred-MB instance backup never has to be buffered into a Blob in memory first.
// The name comes from the server's own listing and is re-validated server-side, so this
// builds no path the API would not have accepted anyway.
const downloadBackup = (name: string) => {
  window.location.assign(`/v1/system/backups/${encodeURIComponent(name)}`);
};

const BackupPage = () => {
  const queryClient = useQueryClient();
  const list = systemApi.useSystemBackupsList();
  const entries = useSettingsEntries();
  const [error, setError] = useState<string | null>(null);

  const backUpNow = systemApi.useSystemBackupsRun({
    mutation: {
      onSuccess: () => {
        setError(null);
        void queryClient.invalidateQueries({
          queryKey: systemApi.getSystemBackupsListQueryKey(),
        });
      },
      onError: () =>
        setError("The backup could not be written. Check that the backup directory is writable."),
    },
  });

  const handleBackUpNow = useCallback(() => {
    backUpNow.mutate();
  }, [backUpNow]);

  if (list.isError) {
    return <ErrorState error={list.error} onRetry={() => void list.refetch()} />;
  }
  if (list.data?.status !== 200) return null;

  return (
    <SettingsPage
      title="Backup"
      description="Schedule and keep downloadable instance backups. Restore remains an offline command-line operation."
      entries={entries}
      blocks={[
        {
          group: "backup",
          title: "Automatic backups",
          keys: ["backup.schedule", "backup.retain", "backup.dir"],
        },
      ]}
      footer={
        <div className="flex flex-col gap-3">
          <aside className="rounded-lg border border-caution/40 bg-caution/5 p-4 text-sm">
            <h2 className="font-medium">The image volume is separate</h2>
            <p className="mt-1 text-muted-foreground">
              This backup contains the database, including settings, channels, people, and generated secrets.
              It does not copy image files or operator-uploaded artwork.
            </p>
          </aside>
          <BackupPanel
            list={list.data.data}
            onBackUpNow={handleBackUpNow}
            onDownload={downloadBackup}
            pending={backUpNow.isPending}
            error={error}
          />
        </div>
      }
    />
  );
};

const Route = createFileRoute("/_authed/settings/system/backup")({
  component: BackupPage,
});

export { Route };
