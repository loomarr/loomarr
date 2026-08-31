import { createFileRoute } from "@tanstack/react-router";
import { EmailTestPanel } from "@/settings/email-test-panel";
import { NotificationReadiness } from "@/settings/notification-readiness";
import { SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";

const NotificationSettings = () => {
  const entries = useSettingsEntries();
  return (
    <SettingsPage
      title="Notifications"
      description="Set up the ways people receive invitations and recover access to Loomarr."
      entries={entries}
      blocks={[
        {
          group: "notifications",
          title: "Share invitation and recovery links",
          description:
            "Use the browser address recipients can actually reach. Loomarr uses it for copied links and QR codes and never guesses it from an administrator's browser.",
          keys: ["access.public_url"],
          surface: "card",
        },
        {
          group: "notifications",
          title: "Email account messages",
          description:
            "Optional. Send invitations and password recovery through any SMTP provider. STARTTLS is recommended and never falls back to cleartext.",
          keys: [
            "notifications.email.enabled",
            "notifications.email.from_address",
            "notifications.email.from_name",
            "notifications.smtp.host",
            "notifications.smtp.port",
            "notifications.smtp.security",
            "notifications.smtp.username",
            "notifications.smtp.password",
          ],
          surface: "card",
        },
      ]}
      footer={({ liveValue }) =>
        liveValue("notifications.email.enabled") === "true" ? <EmailTestPanel /> : null
      }
    >
      {({ liveValue }) => <NotificationReadiness liveValue={liveValue} />}
    </SettingsPage>
  );
};

const Route = createFileRoute("/_authed/settings/notifications")({
  component: NotificationSettings,
});

export { Route };
