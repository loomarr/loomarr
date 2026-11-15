import { Action, Card, Stack, Text } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import { readSuggestionDraft } from "../suggestion-draft";

const SuggestionDraftReturn = () => {
  const draft = readSuggestionDraft();
  if (!draft) return null;

  return (
    <Card borderRadius={8} render="aside">
      <Stack
        align="center"
        direction="row"
        gap="control"
        justify="between"
        paddingHorizontal={16}
        paddingVertical={12}
        wrap
      >
        <div>
          <Text as="p" textRole="cardLabel">
            Your channel draft is saved
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            Finish setup, then return without retyping it.
          </Text>
        </div>
        <Action accessibilityRole="link" render={<Link to="/requests/new" />} variant="outline">
          Return to channel
        </Action>
      </Stack>
    </Card>
  );
};

export { SuggestionDraftReturn };
