import * as libraryApi from "@loomarr/api/endpoints/library";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { LibrarySettings } from "@loomarr/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";

// Settings → Library's media folders (#2151 decisions 6 and 29). The server discovers folders from
// the media server, Sonarr and Radarr and checks each one; this page shows them and records the
// maintainer's choices. A page opened with no folders recorded looks for them once by itself.

const SettingsLibrary = () => {
  const queryClient = useQueryClient();
  const roots = libraryApi.useListLibraryRoots();
  const status = libraryApi.useLibraryStatus();
  const refresh = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: libraryApi.getListLibraryRootsQueryKey() }),
      queryClient.invalidateQueries({ queryKey: libraryApi.getLibraryStatusQueryKey() }),
    ]);
  const mutation = { mutation: { onSettled: refresh } };
  const discover = libraryApi.useDiscoverLibraryRoots(mutation);
  const setInUse = libraryApi.useSetLibraryRootInUse(mutation);
  const add = libraryApi.useAddLibraryRoot(mutation);
  const map = libraryApi.useSetLibraryPathMapping(mutation);
  const rescan = libraryApi.useRescanLibrary(mutation);
  const confirmRemoval = libraryApi.useConfirmLibraryRemoval(mutation);

  const body = unwrap(roots.data);
  const current = unwrap(status.data);
  const health = current?.health;

  // Look for folders once when none were ever recorded.
  const looked = useRef(false);
  useEffect(() => {
    if (body && body.roots.length === 0 && body.pathMap.length === 0 && !looked.current) {
      looked.current = true;
      discover.mutate();
    }
  }, [body, discover]);

  const error =
    roots.error ??
    status.error ??
    discover.error ??
    setInUse.error ??
    map.error ??
    rescan.error ??
    confirmRemoval.error;
  if (error && !body) return <ErrorState error={error} onRetry={() => void refresh()} />;
  if (!body || !current || !health) return null;

  return (
    <>
      {error ? <ErrorState error={error} /> : null}
      <LibrarySettings
        checking={discover.isPending}
        folders={body.roots}
        health={health}
        held={current.held}
        hooks={current.hooks}
        scan={current.scan}
        onRescan={() => rescan.mutate()}
        onConfirmRemoval={(path) => confirmRemoval.mutate({ data: { path } })}
        onAddFolder={async (path) => {
          try {
            await add.mutateAsync({ data: { path } });
            return undefined;
          } catch (err) {
            const problem = toProblem(err);
            return [problem.title, problem.detail].filter(Boolean).join(". ") || "Couldn't add the folder.";
          }
        }}
        onCheckAgain={() => discover.mutate()}
        onMapFolder={(row) => map.mutate({ data: row })}
        onUseFolder={(path, inUse) => setInUse.mutate({ data: { path, inUse } })}
        pathMap={body.pathMap}
      />
    </>
  );
};

export { SettingsLibrary };
