import type { PairingState } from "@loomarr/core/pairing";

type AwaitingApproval = Extract<PairingState, { status: "awaiting-approval" }>;

type PhoneApprovalProps = {
  /** "Safari" on iPhone; Android's Custom Tab is "your browser". */
  browserName: string;
  countdownLabel: string;
  onApprove: () => void;
  onChooseServer: () => void;
  state: AwaitingApproval;
  urgent: boolean;
};

export type { PhoneApprovalProps };
