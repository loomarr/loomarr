import { fillerApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";

// The filler page's cache-invalidation vocabulary, in one place.
//
// ⚠ These three keys travel together for a REASON, and that reason is easy to forget at a call
// site: filing, holding, removing or retagging a clip moves it between the Incoming queue and
// the Catalog, and both of those change what the pool strip reports. Invalidating only the clip
// list leaves the row sitting in Incoming until a reload — which is what "it worked but the UI
// lied" looks like.
//
// The pair was written out four times in filler-page alone, which is three chances to forget the
// third key.
const useFillerInvalidate = () => {
  const queryClient = useQueryClient();

  // The clip catalog alone — a change that cannot move a clip between queues (a retag inside
  // the Catalog, say).
  const invalidateCatalog = () => {
    void queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerQueryKey() });
  };

  // The two queue-shaped views: what is waiting, and what the catalog can cover.
  const invalidateQueues = () => {
    void queryClient.invalidateQueries({ queryKey: fillerApi.getFillerIncomingQueryKey() });
    void queryClient.invalidateQueries({ queryKey: fillerApi.getFillerPoolQueryKey() });
  };

  // ⚠ The default for any clip LIFECYCLE write (file, hold, remove, confirm-era). If you are
  // unsure which of the three above you need, you need this one: over-invalidating costs a
  // refetch, under-invalidating shows the operator a stale queue.
  const invalidateLifecycle = () => {
    invalidateCatalog();
    invalidateQueues();
  };

  return { invalidateCatalog, invalidateLifecycle, invalidateQueues };
};

export { useFillerInvalidate };
