import { Mail, QrCode } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { InvitationRosterProps } from "./invitation-roster.type";

const deliveryCopy = {
  queued: { label: "Email queued", hint: "Loomarr will send this account message shortly." },
  sending: { label: "Sending email", hint: "The SMTP server is receiving this account message." },
  delivered: { label: "Email delivered", hint: "The SMTP server accepted the invitation message." },
  failed: { label: "Email failed", hint: "Email delivery failed. Retry it or use copy or QR sharing." },
  suppressed: { label: "Email unavailable", hint: "Email is disabled or has no usable destination." },
} as const;

const outcomeHint = (outcome?: string) => {
  switch (outcome) {
    case "recipient_rejected":
      return "The SMTP server rejected that recipient. Check the contact address, then retry or use copy or QR sharing.";
    case "configuration_invalid":
    case "means_unavailable":
      return "Email configuration needs attention. Copy or QR sharing remains available.";
    case "transport_unavailable":
      return "The SMTP server could not be reached. Loomarr retries known pre-acceptance failures automatically.";
    case "acceptance_ambiguous":
    case "worker_interrupted":
      return "The message may have been accepted. Check the inbox before explicitly resending it.";
    case "delivery_disabled":
      return "Email delivery is disabled. Copy or QR sharing remains available.";
    case "destination_unavailable":
      return "Add a contact address before sending email.";
    default:
      return undefined;
  }
};

const InvitationRoster = ({ invitations, sendingId, onSendEmail, onShare }: InvitationRosterProps) => (
  <section aria-labelledby="invitation-roster-title" className="flex flex-col gap-3">
    <div>
      <h2 id="invitation-roster-title" className="font-semibold text-lg">
        Invitations
      </h2>
      <p className="text-muted-foreground text-sm">
        Pending admission decisions and the latest attempt to deliver each one.
      </p>
    </div>

    {invitations === undefined ? (
      <div
        className="h-24 animate-pulse rounded-lg border border-border bg-card"
        role="status"
        aria-label="Loading invitations"
      />
    ) : invitations.length === 0 ? (
      <div className="rounded-lg border border-border bg-card p-5 text-muted-foreground text-sm">
        No invitations yet. Directly created and imported people continue to appear below.
      </div>
    ) : (
      <ul className="grid gap-3">
        {invitations.map((value) => {
          const name = value.displayName || value.username || value.libraryUserId || "Reserved person";
          const delivery = value.emailDelivery;
          const copy = delivery ? deliveryCopy[delivery.status] : undefined;
          const pending = value.status === "pending";
          const canSend = pending && Boolean(value.contactAddress);
          const retry = delivery?.status === "failed" || delivery?.status === "suppressed";
          const action = retry
            ? "Retry email"
            : delivery?.status === "delivered"
              ? "Resend email"
              : "Send email";
          const hint = outcomeHint(delivery?.outcome) ?? copy?.hint;

          return (
            <li key={value.id} className="rounded-lg border border-border bg-card p-4">
              <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div className="min-w-0 space-y-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">{name}</span>
                    <Badge variant={value.role === "admin" ? "tune" : "neutral"}>{value.role}</Badge>
                    <Badge variant={value.status === "revoked" ? "onair" : "neutral"}>
                      {value.status === "redeemed" ? "Redeemed" : value.status}
                    </Badge>
                    {copy && (
                      <Badge variant={delivery?.status === "failed" ? "onair" : "neutral"}>
                        {copy.label}
                      </Badge>
                    )}
                  </div>
                  <p className="text-sm text-static-400">
                    {value.contactAddress?.email ?? "Add a contact address before sending email."}
                  </p>
                  {hint && <p className="max-w-3xl text-muted-foreground text-sm">{hint}</p>}
                </div>

                {pending && (
                  <div className="flex flex-wrap gap-2">
                    {canSend && (
                      <Button
                        type="button"
                        size="sm"
                        variant={retry ? "default" : "outline"}
                        disabled={
                          sendingId === value.id ||
                          delivery?.status === "queued" ||
                          delivery?.status === "sending"
                        }
                        aria-label={`${action} to ${name}`}
                        onClick={() => onSendEmail(value.id)}
                      >
                        <Mail aria-hidden />
                        {sendingId === value.id ? "Queuing…" : action}
                      </Button>
                    )}
                    <Button type="button" size="sm" variant="ghost" onClick={() => onShare(value)}>
                      <QrCode className="size-4" aria-hidden /> Share QR or link
                    </Button>
                  </div>
                )}
              </div>
            </li>
          );
        })}
      </ul>
    )}
  </section>
);

export { InvitationRoster };
