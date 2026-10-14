import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState } from "react";

// useRestartWatch tracks a restart from the moment it is accepted until the app answers
// again (§9.2, V13).
//
// ⚠ **It polls `/v1/healthz` with a bare fetch, not through the generated client.** During a
// restart the server is genuinely down for a moment, so every request fails — and healthz is the
// one route that is unauthenticated and has no dependencies, which makes "it answered" mean "the
// new generation is serving" rather than "some subsystem happens to be up". Going through
// TanStack Query would add caching and a retry policy to a poll whose whole job is to observe
// failure honestly.
//
// ⚠ **`back` lingers deliberately.** Clearing the banner the instant health returns makes
// a successful restart indistinguishable from one that never happened — the operator
// clicks, something flickers, and they are left unsure. A brief confirmation is the
// difference between a silent success and a reassuring one.

const POLL_MS = 500;
const CONFIRM_MS = 2500;
// A restart that has not answered in this long is not coming back on its own — the
// operator needs to hear that rather than watch a spinner forever.
const GIVE_UP_MS = 30_000;

interface RestartWatch {
  restarting: boolean;
  justCameBack: boolean;
  /** Non-null when the app never came back, so the caller can say so. */
  failed: string | null;
  /** Call when the server has accepted the restart (the 202). */
  begin: () => void;
}

const useRestartWatch = (): RestartWatch => {
  const queryClient = useQueryClient();
  const [restarting, setRestarting] = useState(false);
  const [justCameBack, setJustCameBack] = useState(false);
  const [failed, setFailed] = useState<string | null>(null);
  // Timers are held so a component unmounting mid-restart cannot leave them running —
  // the same leak discipline the backend's generation loop is gated on.
  const timers = useRef<ReturnType<typeof setTimeout>[]>([]);

  useEffect(
    () => () => {
      for (const t of timers.current) clearTimeout(t);
    },
    [],
  );

  const begin = useCallback(() => {
    setFailed(null);
    setJustCameBack(false);
    setRestarting(true);

    const startedAt = Date.now();

    const poll = async () => {
      try {
        // Deliberately a bare fetch rather than the generated client: this polls ACROSS a
        // restart, so it must answer while the app is mid-reboot and must not go through
        // TanStack Query's cache or retry policy. `/v1/healthz` is the canonical path (the
        // probes moved under /v1 with the rest of the ops surface); the bare `/healthz` alias
        // still answers, for consumers configured outside this repo.
        const res = await fetch("/v1/healthz", { cache: "no-store" });
        if (res.ok) {
          setRestarting(false);
          setJustCameBack(true);
          // Every cached query was answered by the OLD generation, and some of it (the
          // restart cost, the version's startedAt) is now stale by definition.
          void queryClient.invalidateQueries();
          timers.current.push(setTimeout(() => setJustCameBack(false), CONFIRM_MS));
          return;
        }
      } catch {
        // Expected while the server is down — this is the normal path, not an error.
      }
      if (Date.now() - startedAt > GIVE_UP_MS) {
        setRestarting(false);
        setFailed("Loomarr hasn't come back. Check the container or service.");
        return;
      }
      timers.current.push(setTimeout(poll, POLL_MS));
    };

    // A first poll after one interval, not immediately: the old generation is often still
    // answering for a few milliseconds after the 202, and an instant success would report
    // "back" without anything having restarted.
    timers.current.push(setTimeout(poll, POLL_MS));
  }, [queryClient]);

  return { restarting, justCameBack, failed, begin };
};

export { useRestartWatch };
