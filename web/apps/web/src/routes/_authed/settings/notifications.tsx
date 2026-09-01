import { createFileRoute } from "@tanstack/react-router";
import { NotificationDestinationsPanel } from "@/settings/notification-destinations-panel";
import { SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";

const NotificationSettings = () => {
  const entries = useSettingsEntries();
  return (
    <SettingsPage
      title="Notifications"
      description="Choose where Loomarr sends account messages and product events."
      entries={entries}
      blocks={[]}
    >
      <NotificationDestinationsPanel />
    </SettingsPage>
  );
};

const Route = createFileRoute("/_authed/settings/notifications")({
  component: NotificationSettings,
});

export { Route };
