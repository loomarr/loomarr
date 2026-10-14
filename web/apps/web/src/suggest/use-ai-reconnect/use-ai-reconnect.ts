import * as settingsApi from "@loomarr/api/endpoints/settings";
import { unwrap } from "@loomarr/api/unwrap";
import { useRef, useState } from "react";

const POLL_MS = 15_000;
const MAX_BACKOFF_MS = 60_000;

// Consecutive failed reads double the wait from POLL_MS up to MAX_BACKOFF_MS; one good read
// resets it. Counted from the query's own update counters, so it holds however often the
// interval callback is asked.
const pollDelay = (failures: number) =>
  failures === 0 ? POLL_MS : Math.min(MAX_BACKOFF_MS, POLL_MS * 2 ** failures);

type Phase = "unknown" | "off" | "on" | "reconnected";

// useAiReconnect answers one question while the manual builder is open: did AI go from
// unavailable to available? The signal is `features.suggestions` from GET /v1/settings — the
// same settings fold the POST /v1/proposals gate uses to answer `feature_not_configured`, so
// the banner can never promise a run the server would refuse. That read is admin-only, so the
// caller enables this for admins alone.
//
// The first fresh read is the baseline: AI already on means there was nothing to reconnect and
// polling stops. Polling also stops for good once the transition is seen. TanStack Query skips
// interval refetches while the document is hidden (refetchIntervalInBackground is false) and
// stops them when the builder unmounts.
const useAiReconnect = (enabled: boolean): boolean => {
  const [phase, setPhase] = useState<Phase>("unknown");
  // Data cached before the builder opened (e.g. from a Settings visit) is not this session's
  // baseline; only a read that landed after mount counts.
  const openedAt = useRef(Date.now());
  const counts = useRef({ data: 0, errors: 0, failures: 0 });
  const watching = enabled && (phase === "unknown" || phase === "off");

  const settings = settingsApi.useSettingsList({
    query: {
      enabled: watching,
      retry: false,
      refetchOnMount: "always",
      refetchIntervalInBackground: false,
      refetchInterval: (query) => {
        const seen = counts.current;
        if (query.state.dataUpdateCount !== seen.data) {
          seen.data = query.state.dataUpdateCount;
          seen.failures = 0;
        }
        if (query.state.errorUpdateCount !== seen.errors) {
          seen.errors = query.state.errorUpdateCount;
          seen.failures += 1;
        }
        return pollDelay(query.state.status === "error" ? seen.failures : 0);
      },
    },
  });

  const suggestions = unwrap(settings.data, (body) => body.features)?.suggestions;
  const fresh = settings.dataUpdatedAt >= openedAt.current;
  if (watching && fresh && suggestions !== undefined) {
    const next: Phase = !suggestions ? "off" : phase === "off" ? "reconnected" : "on";
    if (next !== phase) setPhase(next);
  }

  return phase === "reconnected";
};

export { MAX_BACKOFF_MS, POLL_MS, pollDelay, useAiReconnect };
