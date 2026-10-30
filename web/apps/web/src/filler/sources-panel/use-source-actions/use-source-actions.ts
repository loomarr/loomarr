import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FetchFillerSourceOutputBody } from "@loomarr/api/models/fetchFillerSourceOutputBody";
import type { SetFillerSourceEnabledInputBody } from "@loomarr/api/models/setFillerSourceEnabledInputBody";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

interface UpdateSourceCallbacks {
  onSuccess?: () => void;
  onError?: (error: { detail?: string }) => void;
  onSettled?: () => void;
}

// The mutations that act on a registered source, in one place so the list and the source drawer
// share a single pending/error state per action: the list's error line must show a failure from the
// drawer's "Save", and a switch pressed in the list must disable the drawer's save button.
//
// ⚠ None of these invalidate the clip list — except a fetch, which can add clips. Switching,
// forgetting or re-scheduling a source forgets or withdraws the REGISTRATION, never the files,
// which are real and may already be tagged and pinned into a channel.
const useSourceActions = ({ onSourceRemoved }: { onSourceRemoved: () => void }) => {
  const queryClient = useQueryClient();
  const invalidateSources = () =>
    queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerSourcesQueryKey() });

  const fetchSource = fillerApi.useFetchFillerSource({
    mutation: {
      onSuccess: () => {
        // Both: the catalog changed AND the per-source counts derive from it.
        void queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerQueryKey() });
        void invalidateSources();
      },
    },
  });

  // Switching a source on or off (V35). ⚠ The switch withdraws a source from future scanning,
  // searching and downloading; clips already in the catalog are untouched.
  const [updatingSource, setUpdatingSource] = useState<string>();
  const updateSourceMutation = fillerApi.useSetFillerSourceEnabled({
    mutation: {
      onSettled: () => setUpdatingSource(undefined),
      onSuccess: () => void invalidateSources(),
    },
  });

  const [removingSource, setRemovingSource] = useState<string>();
  const removeSourceMutation = fillerApi.useDeleteFillerSource({
    mutation: {
      onSettled: () => setRemovingSource(undefined),
      onSuccess: () => {
        onSourceRemoved();
        toast.success("Source removed", {
          description: "Clips it already brought in stay in your catalog.",
        });
        void invalidateSources();
      },
    },
  });

  const [togglingProvider, setTogglingProvider] = useState<"archive" | "youtube">();
  const toggleProviderMutation = fillerApi.useSetFillerProviderEnabled({
    mutation: {
      onSettled: () => setTogglingProvider(undefined),
      // Keep the provider in its pending folded state until the refreshed read model arrives.
      // Clearing `togglingProvider` before this promise settles would briefly reopen enabled-looking
      // children between the PATCH response and the GET that reports `providerEnabled: false`.
      onSuccess: () => invalidateSources(),
    },
  });

  return {
    /** First failure among the actions, as the detail line the source list shows. */
    error:
      updateSourceMutation.error?.detail ??
      fetchSource.error?.detail ??
      removeSourceMutation.error?.detail ??
      toggleProviderMutation.error?.detail,
    fetchSource: {
      pending: fetchSource.isPending,
      error: fetchSource.error,
      run: (id: string, onResult: (result: FetchFillerSourceOutputBody | undefined) => void) =>
        fetchSource.mutate(
          { params: { id } },
          { onSuccess: (response) => onResult(unwrap(response, (body) => body)) },
        ),
    },
    updateSource: (id: string, data: SetFillerSourceEnabledInputBody, callbacks?: UpdateSourceCallbacks) => {
      setUpdatingSource(id);
      updateSourceMutation.mutate({ id, data }, callbacks);
    },
    updating: updateSourceMutation.isPending,
    /** The source being updated right now, if any. */
    updatingSourceID: updateSourceMutation.isPending ? updatingSource : undefined,
    removeSource: (id: string) => {
      setRemovingSource(id);
      removeSourceMutation.mutate({ id });
    },
    removing: removeSourceMutation.isPending,
    removingSourceID: removeSourceMutation.isPending ? removingSource : undefined,
    toggleProvider: (kind: "archive" | "youtube", enabled: boolean) => {
      setTogglingProvider(kind);
      toggleProviderMutation.mutate({ kind, data: { enabled } });
    },
    togglingProvider: toggleProviderMutation.isPending ? togglingProvider : undefined,
  };
};

type SourceActions = ReturnType<typeof useSourceActions>;

export type { SourceActions };
export { useSourceActions };
