import { Action } from "@loomarr/design-system";

import { usePhoneDensity } from "../../phone-density";

const CheckAgain = ({ checking, onPress }: { checking: boolean; onPress: () => void }) => {
  const density = usePhoneDensity();
  return (
    <Action density={density} disabled={checking} onPress={onPress} tone="primary">
      {checking ? "Checking…" : "Check again"}
    </Action>
  );
};

export { CheckAgain };
