import { Action, Grid, Stack, Text } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import type { HomeStripProps } from "./home-strip.type";

// HomeStrip — the whole-page states at the top of Home. Services not answering, requests needing a
// decision and a pending restart each have their own surface — the Requests nav badge and Settings →
// This server's Diagnostics (#1659, Q-H7) — and the channel count is the wall's own header (#2764),
// so with channels to show this renders nothing.
//
// Loading, zero-channels and initial-error are whole-page states (the record's state contract):
// HomePage renders nothing else alongside them, which is why this component owns their copy too
// rather than leaving the caller to duplicate it per state.
const HomeStrip = ({ state, isAdmin, onRetry }: HomeStripProps) => {
  if (state === "loading") {
    return (
      <section aria-busy="true">
        <Text as="p" textRole="compact" tone="secondary">
          Loading your channels…
        </Text>
        <Grid aria-hidden columns={{ base: 1, sm: 2, lg: 3 }} gap="3" marginTop={16}>
          {[0, 1, 2].map((i) => (
            <Stack
              aspectRatio={16 / 9}
              backgroundColor="$surfaceElevated"
              borderRadius={6}
              data-pulse="loading"
              key={i}
              width="100%"
            />
          ))}
        </Grid>
      </section>
    );
  }

  if (state === "error") {
    return (
      <Stack align="start" gap="4" maxWidth={460} paddingVertical={40} render="section">
        <Text as="h2" strong textRole="headline">
          Couldn’t load your channels.
        </Text>
        <Action onPress={onRetry}>Try again</Action>
      </Stack>
    );
  }

  if (state === "empty") {
    return (
      <Stack align="start" gap="4" maxWidth={460} paddingVertical={40} render="section">
        <Text as="h2" strong textRole="headline">
          Your first channel starts here
        </Text>
        <Text as="p" textRole="body" tone="secondary">
          {isAdmin ? "Create" : "Request"} a channel to start watching.
        </Text>
        <Action accessibilityRole="link" render={<Link to="/requests/new" />}>
          {isAdmin ? "Add your first channel" : "Request a channel"}
        </Action>
      </Stack>
    );
  }

  return null;
};

export { HomeStrip };
