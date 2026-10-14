import type { CreateInvitationInputBody } from "@loomarr/api/models/createInvitationInputBody";
import type { ImportCandidate } from "@loomarr/api/models/importCandidate";
import type { InvitationBody } from "@loomarr/api/models/invitationBody";
import type { IssueInvitationGrantInputBodyConveyance } from "@loomarr/api/models/issueInvitationGrantInputBodyConveyance";
import type { IssueInvitationGrantOutputBody } from "@loomarr/api/models/issueInvitationGrantOutputBody";

interface InvitationDialogProps {
  candidates?: ImportCandidate[];
  defaultOpen?: boolean;
  existing?: InvitationBody;
  libraryAvailable: boolean;
  open?: boolean;
  portalContainer?: HTMLElement | ShadowRoot | null;
  onCreate: (input: CreateInvitationInputBody) => Promise<InvitationBody>;
  onIssueGrant: (
    invitationId: string,
    conveyance: IssueInvitationGrantInputBodyConveyance,
  ) => Promise<IssueInvitationGrantOutputBody>;
  onOpenChange?: (open: boolean) => void;
  onRevoke: (invitationId: string) => Promise<unknown>;
  onSendEmail?: (invitationId: string) => Promise<unknown>;
}

export type { InvitationDialogProps };
