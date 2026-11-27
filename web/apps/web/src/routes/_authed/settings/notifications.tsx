import * as notificationsApi from "@loomarr/api/endpoints/notifications";
import { createFileRoute } from "@tanstack/react-router";
import { viewerIsAdmin } from "@/auth/me-query";
import { NotificationDestinationsPanel } from "@/settings/notification-destinations-panel";
import { SettingsPage } from "@/settings/settings-page";

const NotificationSettings = () => {
  return (
    <SettingsPage
      title="Notifications"
      description="Choose where Loomarr sends account messages and product events."
      entries={[]}
      blocks={[]}
    >
      <NotificationDestinationsPanel />
    </SettingsPage>
  );
};

const Route = createFileRoute("/_authed/settings/notifications")({
  component: NotificationSettings,
  // The destination kinds and the saved destinations the panel is made of. Admin-only, like the
  // page; each answers for itself in the panel, so they are warmed.
  loader: async ({ context: { queryClient } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await Promise.all([
      queryClient.prefetchQuery(notificationsApi.getNotificationProviderTypesListQueryOptions()),
      queryClient.prefetchQuery(notificationsApi.getNotificationProvidersListQueryOptions()),
    ]);
  },
});

export { Route };
