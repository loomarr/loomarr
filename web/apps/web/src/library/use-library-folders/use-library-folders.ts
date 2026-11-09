import type { LibraryStatusOutputBody } from "@loomarr/api";
import * as libraryApi from "@loomarr/api/endpoints/library";
import { LibraryScanDTOStage } from "@loomarr/api/models/libraryScanDTOStage";
import { ListLibraryItemsSort } from "@loomarr/api/models/listLibraryItemsSort";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";

// THE MEDIA FOLDERS (#2151 decisions 6, 29 and 30): the folders the server found through the media
// server, Sonarr and Radarr, and the library's status, with the maintainer's choices. Settings →
// Library and the wizard's Library step both read them here. With no folders ever recorded, it
// looks for them once by itself.

// While new files are read, the status and the titles ready so far refetch every 5 s (decision 35),
// so the progress and "Ready so far" (decision 29) move on their own.
const READING_REFETCH_MS = 5000;
const READY_SO_FAR = 5;
const reading = (status: LibraryStatusOutputBody | undefined) =>
  status?.scan.stage === LibraryScanDTOStage.reading;

const useLibraryFolders = ({ enabled = true }: { enabled?: boolean } = {}) => {
  const queryClient = useQueryClient();
  const roots = libraryApi.useListLibraryRoots({ query: { enabled } });
  const status = libraryApi.useLibraryStatus({
    query: {
      enabled,
      refetchInterval: (query) => (reading(unwrap(query.state.data)) ? READING_REFETCH_MS : false),
    },
  });
  const isReading = reading(unwrap(status.data));
  const ready = libraryApi.useListLibraryItems(
    { ready: true, sort: ListLibraryItemsSort.recent, limit: READY_SO_FAR },
    { query: { enabled: enabled && isReading, refetchInterval: isReading ? READING_REFETCH_MS : false } },
  );
  const refresh = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: libraryApi.getListLibraryRootsQueryKey() }),
      queryClient.invalidateQueries({ queryKey: libraryApi.getLibraryStatusQueryKey() }),
    ]);
  const mutation = { mutation: { onSettled: refresh } };
  const discover = libraryApi.useDiscoverLibraryRoots(mutation);
  const setInUse = libraryApi.useSetLibraryRootInUse(mutation);
  const add = libraryApi.useAddLibraryRoot(mutation);

  const body = unwrap(roots.data);
  const current = unwrap(status.data);

  // Look for folders once when none were ever recorded.
  const looked = useRef(false);
  useEffect(() => {
    if (body && body.roots.length === 0 && body.pathMap.length === 0 && !looked.current) {
      looked.current = true;
      discover.mutate();
    }
  }, [body, discover]);

  // Resolves to a sentence saying why the folder was refused, or undefined when it was added.
  const onAddFolder = async (path: string) => {
    try {
      await add.mutateAsync({ data: { path } });
      return undefined;
    } catch (err) {
      const problem = toProblem(err);
      return [problem.title, problem.detail].filter(Boolean).join(". ") || "Couldn't add the folder.";
    }
  };

  return {
    roots: body,
    status: current,
    /** The latest titles that became ready, last first, while files are read; empty otherwise. */
    readySoFar: (isReading ? unwrap(ready.data)?.items : undefined) ?? [],
    error: roots.error ?? status.error ?? discover.error ?? setInUse.error,
    refresh,
    checking: discover.isPending,
    checked: discover.isSuccess,
    onCheckAgain: () => discover.mutate(),
    onUseFolder: (path: string, inUse: boolean) => setInUse.mutate({ data: { path, inUse } }),
    onAddFolder,
  };
};

export { useLibraryFolders };
