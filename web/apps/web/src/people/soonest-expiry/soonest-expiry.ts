import type { InvitationBody } from "@loomarr/api/models/invitationBody";

// soonestExpiry is when the first pending invitation's link runs out, which turns it expired with
// nothing stored; undefined when no link is pending.
const soonestExpiry = (
  invitations: readonly Pick<InvitationBody, "status" | "expiresAt">[],
): number | undefined => {
  let soonest: number | undefined;
  for (const { status, expiresAt } of invitations) {
    if (status === "pending" && (soonest === undefined || expiresAt < soonest)) soonest = expiresAt;
  }
  return soonest;
};

export { soonestExpiry };
