import { Text } from "@loomarr/design-system";

// The line a card shows under its actions when its answer was not saved.
const NotSaved = () => (
  <Text textRole="body" tone="secondary">
    Couldn't save your answer. Try again in a moment.
  </Text>
);

export { NotSaved };
