import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as jobsApi from "@loomarr/api/endpoints/jobs";
import * as notificationsApi from "@loomarr/api/endpoints/notifications";
import * as systemApi from "@loomarr/api/endpoints/system";
import type { EnsureQueryDataOptions, QueryKey } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { viewerIsAdmin } from "@/auth/me-query";
import { SettingsIndexPage } from "@/settings/settings-index-page";

const Route = createFileRoute("/_authed/settings/")({
  component: SettingsIndexPage,
  // The reads behind each card's status word (see `useSettingsStatus`). A card whose check fails
  // settles without a problem, so a failed read is swallowed rather than awaited into an error view.
  // The admin-only reads are skipped for a member, as the page skips them.
  //
  // ⚠ Await a read only when it has NO data. A cached-but-stale read is shown at once and refreshed
  // behind the page (`revalidateIfStale`). Awaiting a `prefetchQuery` instead re-fetched whichever
  // read had gone stale on every return visit and held the navigation for that round trip: warm
  // Settings landed at ~100 ms (the e2e API delay) whenever one had, against a 70 ms budget.
  loader: async ({ context: { queryClient } }) => {
    const isAdmin = await viewerIsAdmin(queryClient);
    const warm = <TQueryFnData, TError, TData, TQueryKey extends QueryKey>(
      options: EnsureQueryDataOptions<TQueryFnData, TError, TData, TQueryKey>,
    ) => queryClient.ensureQueryData({ ...options, revalidateIfStale: true }).catch(() => undefined);
    await Promise.all([
      warm(notificationsApi.getNotificationProvidersListQueryOptions({ query: { retry: false } })),
      isAdmin ? warm(diagnosticsApi.getGetCurrentHealthQueryOptions()) : undefined,
      isAdmin ? warm(jobsApi.getJobsListQueryOptions()) : undefined,
      isAdmin ? warm(systemApi.getSystemBackupsListQueryOptions()) : undefined,
      isAdmin ? warm(fillerApi.getFillerReadinessQueryOptions()) : undefined,
    ]);
  },
});

export { Route };
