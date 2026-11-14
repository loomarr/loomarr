import { Action, Stack, Text } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import { readSuggestionDraft } from "../suggestion-draft";

const SuggestionDraftReturn = () => {
  const draft = readSuggestionDraft();
  if (!draft) return null;

  return (
    <Stack
      align="center"
      className="border-border bg-muted/40 rounded-lg border"
      direction="row"
      gap="control"
      justify="between"
      paddingHorizontal={16}
      paddingVertical={12}
      render="aside"
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
  );
};

export { SuggestionDraftReturn };
