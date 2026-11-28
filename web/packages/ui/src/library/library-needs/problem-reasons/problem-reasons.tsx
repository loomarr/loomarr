import { Text } from "@loomarr/design-system";

import { Stack } from "../../library-poster";
import type { ProblemReason } from "../problem-entries";

// What is wrong with a title, each as a headline and its one reason. `lead` makes the headlines the
// card's titles, for a card that asks nothing else.
const ProblemReasons = ({ reasons, lead }: { reasons: readonly ProblemReason[]; lead?: boolean }) => (
  <>
    {reasons.map((r) => (
      <Stack gap={4} key={r.key}>
        <Text strong={!lead} textRole={lead ? "title" : "body"}>
          {r.headline}
        </Text>
        <Text textRole="body" tone="secondary">
          {r.reason}
        </Text>
      </Stack>
    ))}
  </>
);

export { ProblemReasons };
