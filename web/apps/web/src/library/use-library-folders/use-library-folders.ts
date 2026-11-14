import * as libraryApi from "@loomarr/api/endpoints/library";
import { LibraryScanDTOStep } from "@loomarr/api/models/libraryScanDTOStep";
import type { LibraryStatusOutputBody } from "@loomarr/api/models/libraryStatusOutputBody";
import { ListLibraryItemsSort } from "@loomarr/api/models/listLibraryItemsSort";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { useLibraryScanFrames } from "../use-library-scan-frames";

// THE MEDIA FOLDERS (#2151 decisions 6, 29 and 30): the folders the server found through the media
// server, Sonarr and Radarr, and the library's status, with the maintainer's choices. Settings →
// Library and the wizard's Library step both read them here. With no folders ever recorded, it
// looks for them once by itself.

// While an import runs, its progress arrives as library_scan frames, written into the status read,
// and "Found so far" (decision 29) refetches on each library_titles frame: both move on their own,
// with nothing polling. It lists the latest titles written, ready or not, through every step, so a
// title shows by name from the walk on and turns into its poster once it is ready.
const FOUND_SO_FAR = 5;
const running = (status: LibraryStatusOutputBody | undefined) =>
  status !== undefined && status.scan.step !== LibraryScanDTOStep.idle;

const useLibraryFolders = ({ enabled = true }: { enabled?: boolean } = {}) => {
  const queryClient = useQueryClient();
  useLibraryScanFrames();
  const roots = libraryApi.useListLibraryRoots({ query: { enabled } });
  const status = libraryApi.useLibraryStatus({ query: { enabled } });
  const isRunning = running(unwrap(status.data));
  const found = libraryApi.useListLibraryItems(
    { sort: ListLibraryItemsSort.recent, limit: FOUND_SO_FAR },
    { query: { enabled: enabled && isRunning } },
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
    /** The latest titles written, ready or not, last first, while an import runs; empty otherwise. */
    readySoFar: (isRunning ? unwrap(found.data)?.items : undefined) ?? [],
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
