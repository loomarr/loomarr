import * as libraryApi from "@loomarr/api/endpoints/library";
import { LibrarySettings, StatePanel } from "@loomarr/ui";
import { useHouseholdTimeZone } from "@/channels/use-household-time-zone";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { readyTitleOf } from "../ready-title";
import { useLibraryFolders } from "../use-library-folders";

// Settings → Library's media folders (#2151 decisions 6 and 29). The server discovers folders from
// the media server, Sonarr and Radarr and checks each one; this page shows them and records the
// maintainer's choices, the path map, rescans and confirmed removals.

const SettingsLibrary = () => {
  const folders = useLibraryFolders();
  const timeZone = useHouseholdTimeZone();
  const mutation = { mutation: { onSettled: folders.refresh } };
  const map = libraryApi.useSetLibraryPathMapping(mutation);
  const rescan = libraryApi.useRescanLibrary(mutation);
  const refresh = libraryApi.useRefreshLibrary(mutation);
  const confirmRemoval = libraryApi.useConfirmLibraryRemoval(mutation);

  const body = folders.roots;
  const current = folders.status;
  const health = current?.health;

  const error = folders.error ?? map.error ?? rescan.error ?? refresh.error ?? confirmRemoval.error;
  if (error && !(body && health)) return <ErrorState error={error} onRetry={() => void folders.refresh()} />;
  if (!body || !current || !health) return <StatePanel kind="loading" title="Loading your media folders" />;

  return (
    <>
      {error ? <ErrorState error={error} /> : null}
      <LibrarySettings
        checking={folders.checking}
        folders={body.roots}
        health={health}
        held={current.held}
        hooks={current.hooks}
        details={current.details}
        scan={current.scan}
        readySoFar={folders.readySoFar.map(readyTitleOf)}
        onRescan={() => rescan.mutate()}
        onRefreshEverything={() => refresh.mutate()}
        onConfirmRemoval={(path) => confirmRemoval.mutate({ data: { path } })}
        onAddFolder={folders.onAddFolder}
        onCheckAgain={folders.onCheckAgain}
        onMapFolder={(row) => map.mutate({ data: row })}
        onUseFolder={folders.onUseFolder}
        pathMap={body.pathMap}
        timeZone={timeZone}
      />
    </>
  );
};

export { SettingsLibrary };
