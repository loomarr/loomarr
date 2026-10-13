import { Action, Badge, Surface, Text } from "@loomarr/design-system";

import type { PhoneApprovalProps } from "./phone-approval.type";

// A phone approves itself (maintainer, 2026-10-13): one button opens the server's `/pair` page with
// the code filled in, in a browser sheet over the app; the shell closes the sheet the moment the
// session pairs. A QR code is for a second device, so the phone has none. The code stays as small
// print for approving from another device, which is still how a signed-out phone gets approved by
// someone else in the household.
const PhoneApproval = ({
  browserName,
  countdownLabel,
  onApprove,
  onChooseServer,
  state,
  urgent,
}: PhoneApprovalProps) => (
  <>
    <Surface backgroundColor="$transparent" borderWidth={0} gap="$inline">
      <Text density="touch" textRole="title">
        Approve this phone
      </Text>
      <Text density="touch" textRole="body" tone="secondary">
        Sign in to Loomarr and approve it. You’ll come straight back here.
      </Text>
    </Surface>
    <Action density="touch" onPress={onApprove}>
      {`Approve in ${browserName}`}
    </Action>
    <Surface backgroundColor="$transparent" borderWidth={0} gap="$inline">
      <Text density="touch" textRole="metadata" tone="muted">
        {`On another device? Go to ${state.verificationUri} and enter`}
      </Text>
      <Text accessibilityLabel={`Pairing code ${state.userCode}`} density="touch" selectable textRole="time">
        {state.userCode}
      </Text>
      {state.refreshedFromExpired ? (
        <Text accessibilityLiveRegion="polite" density="touch" textRole="metadata" tone="muted">
          New code — the last one expired.
        </Text>
      ) : null}
    </Surface>
    <Surface
      alignItems="center"
      backgroundColor="$transparent"
      borderWidth={0}
      flexDirection="row"
      justifyContent="space-between"
    >
      {urgent ? (
        <Badge accessibilityLiveRegion="polite" density="touch" tone="danger">
          {countdownLabel}
        </Badge>
      ) : (
        <Text accessibilityLiveRegion="polite" density="touch" textRole="metadata" tone="muted">
          {countdownLabel}
        </Text>
      )}
      <Action density="touch" onPress={onChooseServer} tone="secondary">
        Use a different server
      </Action>
    </Surface>
  </>
);

export { PhoneApproval };
