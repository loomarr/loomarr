import { createFileRoute } from "@tanstack/react-router";
import { EmailTestPanel } from "@/settings/email-test-panel";
import { SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";

const NotificationSettings = () => (
  <SettingsPage
    title="Notifications"
    description="Choose the address recipients can open and configure how Loomarr sends account messages."
    entries={useSettingsEntries()}
    blocks={[
      {
        group: "notifications",
        title: "Recipient links",
        description:
          "This browser address is used for invitations, password recovery, copied links, and QR codes. It is never inferred from a request or from the administrator's browser.",
        keys: ["access.public_url"],
      },
      {
        group: "notifications",
        title: "Email delivery",
        description:
          "SMTP is provider-neutral: use Postmark, SES, Mailgun, Gmail, or a local relay. STARTTLS is the recommended submission mode and never falls back to cleartext.",
        keys: [
          "notifications.email.enabled",
          "notifications.smtp.host",
          "notifications.smtp.port",
          "notifications.smtp.security",
          "notifications.smtp.username",
          "notifications.smtp.password",
          "notifications.email.from_address",
          "notifications.email.from_name",
        ],
      },
    ]}
    footer={<EmailTestPanel />}
  />
);

const Route = createFileRoute("/_authed/settings/notifications")({
  component: NotificationSettings,
});

export { Route };
