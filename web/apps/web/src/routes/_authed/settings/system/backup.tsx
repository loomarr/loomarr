import { systemApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useCallback, useState } from "react";
import { BackupPanel, ErrorState } from "@/components/loomarr";

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
    <div className="overflow-y-auto p-6">
      <BackupPanel
        list={list.data.data}
        onBackUpNow={handleBackUpNow}
        onDownload={downloadBackup}
        pending={backUpNow.isPending}
        error={error}
      />
    </div>
  );
};

const Route = createFileRoute("/_authed/settings/system/backup")({
  component: BackupPage,
});

export { Route };
