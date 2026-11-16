import { Action } from "@loomarr/design-system";

const CheckAgain = ({ checking, onPress }: { checking: boolean; onPress: () => void }) => (
  <Action disabled={checking} onPress={onPress} tone="primary">
    {checking ? "Checking…" : "Check again"}
  </Action>
);

export { CheckAgain };
