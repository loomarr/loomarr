import * as libraryApi from "@loomarr/api/endpoints/library";
import type { LibraryScanEvent } from "@loomarr/core/events";
import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { useLoomarrEventListener } from "@/events/events-provider";

// A LIBRARY IMPORT'S PROGRESS, LIVE (#2151): each library_scan frame is the whole of how far the
// import is, so it is written straight into the status read every screen showing it renders from.
// The status GET stays the truth on load and reconnect, and the bus refetches it once the import
// goes idle (EVENT_INVALIDATIONS); nothing polls while it runs.

/** Writes one frame into the cached status, if one is cached. */
const applyLibraryScanFrame = (queryClient: QueryClient, frame: LibraryScanEvent) =>
  queryClient.setQueryData<libraryApi.libraryStatusResponse>(libraryApi.getLibraryStatusQueryKey(), (old) =>
    old?.status === 200
      ? {
          ...old,
          data: {
            ...old.data,
            // Spelled out: a frame leaves out what it doesn't know, which must clear the old value.
            scan: { ...old.data.scan, ...frame, current: frame.current, etaSeconds: frame.etaSeconds },
          },
        }
      : old,
  );

/** Keeps the cached library status in step with the import's frames while mounted. */
const useLibraryScanFrames = () => {
  const queryClient = useQueryClient();
  useLoomarrEventListener({ onLibraryScan: (frame) => applyLibraryScanFrame(queryClient, frame) });
};

export { useLibraryScanFrames };
