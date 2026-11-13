import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import type { QueryClient } from "@tanstack/react-query";
import { viewerIsAdmin } from "@/auth/me-query";

// What every Filler page shows around its tab: the status line (the watch summary) and, for an
// admin, the settings list that says whether filler is configured. FillerPage reads exactly these
// two, so the routes' loaders warm them through here and the page opens on its content rather than
// "Loading filler…". Warmed, not awaited to success: a failed read leaves the page to answer as it
// does once open (a member's missing watch line, an admin's "no filler folder configured").
const warmFillerPage = async (queryClient: QueryClient): Promise<void> => {
  const isAdmin = await viewerIsAdmin(queryClient);
  await Promise.all([
    queryClient.prefetchQuery(fillerApi.getFillerWatchQueryOptions()),
    isAdmin ? queryClient.prefetchQuery(settingsApi.getSettingsListQueryOptions()) : undefined,
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

export { warmDefectReport, warmFillerPage };
