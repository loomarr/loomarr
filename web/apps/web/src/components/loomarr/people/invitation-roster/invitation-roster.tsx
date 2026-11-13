import { Action, Stack, StatusLabel, Text } from "@loomarr/design-system";
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
  <Stack aria-labelledby="invitation-roster-title" gap="control" render="section">
    <div>
      <Text as="h2" id="invitation-roster-title" strong textRole="headline">
        Invitations
      </Text>
      <Text as="p" textRole="compact" tone="secondary">
        Invitations you've sent and whether they arrived.
      </Text>
    </div>

    {invitations === undefined ? (
      <div
        className="border-border bg-card h-24 animate-pulse rounded-lg border"
        role="status"
        aria-label="Loading invitations"
      />
    ) : invitations.length === 0 ? (
      <div className="border-border bg-card rounded-lg border p-5">
        <Text as="p" textRole="compact" tone="secondary">
          No invitations yet. Directly created and imported people continue to appear below.
        </Text>
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
            <li key={value.id} className="border-border bg-card rounded-lg border p-4">
              <Stack
                align={{ base: "stretch", sm: "start" }}
                direction={{ base: "column", sm: "row" }}
                gap="4"
                justify={{ base: "start", sm: "between" }}
              >
                <div className="min-w-0 space-y-2">
                  <Stack align="center" direction="row" gap="inline" wrap>
                    <Text as="span" textRole="cardLabel">
                      {name}
                    </Text>
                    <StatusLabel tone={value.role === "admin" ? "notice" : "off"}>
                      {value.role === "admin" ? "Admin" : "Member"}
                    </StatusLabel>
                    <StatusLabel tone={value.status === "revoked" ? "error" : "off"}>
                      {value.status === "redeemed"
                        ? "Redeemed"
                        : `${value.status.charAt(0).toUpperCase()}${value.status.slice(1)}`}
                    </StatusLabel>
                    {copy && (
                      <StatusLabel tone={delivery?.status === "failed" ? "error" : "off"}>
                        {copy.label}
                      </StatusLabel>
                    )}
                  </Stack>
                  <Text as="p" textRole="compact" tone="secondary">
                    {value.contactAddress?.email ?? "Add a contact address before sending email."}
                  </Text>
                  {hint && (
                    <Text as="p" className="max-w-3xl" textRole="compact" tone="secondary">
                      {hint}
                    </Text>
                  )}
                </div>

                {pending && (
                  <Stack direction="row" flexShrink={1} gap="inline" wrap>
                    {canSend && (
                      <Action
                        accessibilityLabel={`${action} to ${name}`}
                        disabled={
                          sendingId === value.id ||
                          delivery?.status === "queued" ||
                          delivery?.status === "sending"
                        }
                        onPress={() => onSendEmail(value.id)}
                        variant={retry ? undefined : "outline"}
                      >
                        {sendingId === value.id ? "Queuing…" : action}
                      </Action>
                    )}
                    <Action onPress={() => onShare(value)} variant="ghost">
                      Share QR or link
                    </Action>
                  </Stack>
                )}
              </Stack>
            </li>
          );
        })}
      </ul>
    )}
  </Stack>
);

export { InvitationRoster };
