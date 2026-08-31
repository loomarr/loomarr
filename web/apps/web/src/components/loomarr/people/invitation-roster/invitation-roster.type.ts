import type { InvitationBody } from "@loomarr/api/models/invitationBody";

interface InvitationRosterProps {
  invitations?: InvitationBody[];
  sendingId?: string;
  onSendEmail: (id: string) => void;
}

export type { InvitationRosterProps };
