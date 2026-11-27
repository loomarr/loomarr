import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import type { QueryClient } from "@tanstack/react-query";
import { viewerIsAdmin } from "@/auth/me-query";

// What every Filler page shows around its tab: the status line (the watch summary) and, for an
// admin, the settings list. FillerPage reads exactly these two. Which one it cannot render without
// depends on who is asking, because it says whether filler is configured: an admin's answer is the
// settings list, a member's is the watch summary (the settings list is admin-only). That one is
// ensured, so a failed read is the route's error view; the watch summary is only a status line for
// an admin, so it is warmed.
const openFillerPage = async (queryClient: QueryClient): Promise<void> => {
  const isAdmin = await viewerIsAdmin(queryClient);
  const watch = fillerApi.getFillerWatchQueryOptions();
  if (!isAdmin) {
    await queryClient.ensureQueryData(watch);
    return;
  }
  await Promise.all([
    queryClient.prefetchQuery(watch),
    queryClient.ensureQueryData(settingsApi.getSettingsListQueryOptions()),
  ]);
};

// The first page of the defect report the Manage summary and the Quality queue both open on, so
// the two read one cache entry (and agree on the count). Admin-only, as the report is.
const warmDefectReport = async (queryClient: QueryClient): Promise<void> => {
  if (!(await viewerIsAdmin(queryClient))) return;
  await queryClient.prefetchQuery(
    fillerApi.getFillerDefectReportQueryOptions({ unlabelled: true, limit: 1, offset: 0 }),
  );
};

export { openFillerPage, warmDefectReport };
