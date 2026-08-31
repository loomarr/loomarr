import type { InvitationPreviewOutputBody } from "@loomarr/api/models/invitationPreviewOutputBody";

interface InvitationRedemptionValues {
  username?: string;
  password: string;
}

interface InvitationJoinProps {
  preview?: InvitationPreviewOutputBody;
  isLoading?: boolean;
  isRedeeming?: boolean;
  error?: unknown;
  onRedeem: (values: InvitationRedemptionValues) => void;
}

export type { InvitationJoinProps, InvitationRedemptionValues };
