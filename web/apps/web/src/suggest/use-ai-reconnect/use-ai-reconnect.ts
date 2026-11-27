import * as settingsApi from "@loomarr/api/endpoints/settings";
import { unwrap } from "@loomarr/api/unwrap";
import { useRef, useState } from "react";

type Phase = "unknown" | "off" | "on" | "reconnected";

// useAiReconnect answers one question while the manual builder is open: did AI go from
// unavailable to available? The signal is `features.suggestions` from GET /v1/settings — the
// same settings fold the POST /v1/proposals gate uses to answer `feature_not_configured`, so
// the banner can never promise a run the server would refuse. That read is admin-only, so the
// caller enables this for admins alone.
//
// The flag is computed from settings alone, so it changes only with a committed settings write,
// and every such write publishes a settings frame that refetches this read (core events). The
// first fresh read is the baseline: AI already on means there was nothing to reconnect, and the
// read is let go once the transition is seen.
const useAiReconnect = (enabled: boolean): boolean => {
  const [phase, setPhase] = useState<Phase>("unknown");
  // Data cached before the builder opened (e.g. from a Settings visit) is not this session's
  // baseline; only a read that landed after mount counts.
  const openedAt = useRef(Date.now());
  const watching = enabled && (phase === "unknown" || phase === "off");

  const settings = settingsApi.useSettingsList({
    query: { enabled: watching, retry: false, refetchOnMount: "always" },
  });

  const suggestions = unwrap(settings.data, (body) => body.features)?.suggestions;
  const fresh = settings.dataUpdatedAt >= openedAt.current;
  if (watching && fresh && suggestions !== undefined) {
    const next: Phase = !suggestions ? "off" : phase === "off" ? "reconnected" : "on";
    if (next !== phase) setPhase(next);
  }

  return phase === "reconnected";
};

export { useAiReconnect };
