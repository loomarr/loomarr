import { Action, Stack, Text } from "@loomarr/design-system";
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
        <div aria-hidden className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {[0, 1, 2].map((i) => (
            <div key={i} className="bg-static-800 aspect-video w-full animate-pulse rounded-md" />
          ))}
        </div>
      </section>
    );
  }

  if (state === "error") {
    return (
      <Stack align="start" className="max-w-[460px] py-10" gap="4" render="section">
        <Text as="h2" strong textRole="headline">
          Couldn’t load your channels.
        </Text>
        <Action onPress={onRetry}>Try again</Action>
      </Stack>
    );
  }

  if (state === "empty") {
    return (
      <Stack align="start" className="max-w-[460px] py-10" gap="4" render="section">
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
