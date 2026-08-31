import { Link2, Mail } from "lucide-react";
import { StatusDot } from "@/components/ui/status-dot";

interface NotificationReadinessProps {
  liveValue: (key: string) => string;
}

const isRecipientOrigin = (raw: string): boolean => {
  try {
    const value = new URL(raw.trim());
    return (
      (value.protocol === "http:" || value.protocol === "https:") &&
      value.host !== "" &&
      value.username === "" &&
      value.password === "" &&
      (value.pathname === "" || value.pathname === "/") &&
      value.search === "" &&
      value.hash === ""
    );
  } catch {
    return false;
  }
};

const isMailbox = (raw: string): boolean => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(raw.trim());

const NotificationReadiness = ({ liveValue }: NotificationReadinessProps) => {
  const linksReady = isRecipientOrigin(liveValue("access.public_url"));
  const emailEnabled = liveValue("notifications.email.enabled") === "true";
  const port = Number(liveValue("notifications.smtp.port"));
  const emailReady =
    linksReady &&
    emailEnabled &&
    liveValue("notifications.smtp.host").trim() !== "" &&
    Number.isInteger(port) &&
    port >= 1 &&
    port <= 65535 &&
    isMailbox(liveValue("notifications.email.from_address"));

  const emailState = emailReady
    ? {
        tone: "ok" as const,
        label: "Ready to test",
        detail: "Invitations and recovery can be sent by email.",
      }
    : emailEnabled
      ? {
          tone: "warn" as const,
          label: "Setup incomplete",
          detail: "Complete the highlighted delivery details, then save and test.",
        }
      : {
          tone: "off" as const,
          label: "Email is off",
          detail: "Copy and QR sharing still work when recipient links are ready.",
        };

  return (
    <section
      aria-labelledby="account-delivery-readiness"
      className="rounded-xl border border-border bg-card p-5 sm:p-6"
    >
      <div className="max-w-2xl">
        <h2 id="account-delivery-readiness" className="font-semibold text-lg">
          Account delivery readiness
        </h2>
        <p className="mt-1 text-muted-foreground text-sm leading-relaxed">
          Invitations and password recovery have two independent paths. Configure only the ones your household
          needs.
        </p>
      </div>

      <div className="mt-5 grid gap-3 lg:grid-cols-2">
        <div className="flex min-w-0 gap-3 rounded-lg border border-border bg-background/40 p-4">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-signal-tint-15 text-signal">
            <Link2 className="size-4" aria-hidden />
          </span>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <StatusDot
                tone={linksReady ? "ok" : "warn"}
                label={linksReady ? "Invitation links ready" : "Invitation links need configuration"}
              />
              <h3 className="font-medium text-sm">Invitation and recovery links</h3>
            </div>
            <p className="mt-1 font-medium text-sm">
              {linksReady ? "Ready to share" : "Recipient address needed"}
            </p>
            <p className="mt-1 text-muted-foreground text-sm leading-relaxed">
              {linksReady
                ? "Copied links and QR codes will open the configured Loomarr address."
                : "Add the browser address recipients can reach to enable copy and QR sharing."}
            </p>
          </div>
        </div>

        <div className="flex min-w-0 gap-3 rounded-lg border border-border bg-background/40 p-4">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-lock/10 text-lock">
            <Mail className="size-4" aria-hidden />
          </span>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <StatusDot tone={emailState.tone} label={`Account email: ${emailState.label}`} />
              <h3 className="font-medium text-sm">Account email</h3>
            </div>
            <p className="mt-1 font-medium text-sm">{emailState.label}</p>
            <p className="mt-1 text-muted-foreground text-sm leading-relaxed">{emailState.detail}</p>
          </div>
        </div>
      </div>
    </section>
  );
};

export { NotificationReadiness };
