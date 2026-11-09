import { Action, Surface, Text } from "@loomarr/design-system";

import type { ConfirmStepProps } from "./confirm-step.type";

// ConfirmStep is the second press of an irreversible action, the same two-step shape as Delete
// channel: the first press only shows this, which states the consequence beside the action itself
// and a way back. Nothing happens until the confirm is pressed.
const ConfirmStep = ({ confirmLabel, disabled = false, message, onCancel, onConfirm }: ConfirmStepProps) => (
  <Surface
    backgroundColor="stateDangerSurface"
    borderColor="stateDanger"
    borderRadius="control"
    gap={8}
    paddingHorizontal={12}
    paddingVertical={12}
  >
    <Text textRole="compact" tone="danger">
      {message}
    </Text>
    <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" flexWrap="wrap" gap={8}>
      <Action disabled={disabled} onPress={onConfirm} variant="destructive">
        {confirmLabel}
      </Action>
      <Action onPress={onCancel} variant="ghost">
        Cancel
      </Action>
    </Surface>
  </Surface>
);

export { ConfirmStep };
