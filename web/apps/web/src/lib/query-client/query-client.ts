import { QueryClient } from "@tanstack/react-query";

// A failed read waits twice as long as the last before it asks again, up to the cap, so a server
// that is down (a run of 503s) is asked a handful of times a minute instead of every second.
const BASE_RETRY_DELAY_MS = 1_000;
const MAX_RETRY_DELAY_MS = 30_000;

/**
 * The wait before retry number `failureCount` (1 for the first): exponential, capped at 30s, with
 * the upper half jittered so many tabs and queries that failed together don't ask together.
 */
const backoff =
  (random: () => number) =>
  (failureCount: number): number => {
    const ceiling = Math.min(MAX_RETRY_DELAY_MS, BASE_RETRY_DELAY_MS * 2 ** Math.max(0, failureCount - 1));
    return Math.round(ceiling / 2 + (random() * ceiling) / 2);
  };

const retryDelay = backoff(Math.random);

// Server state is TanStack Query, SSE-invalidated (no global store, §4.3). Retries
// are conservative — most failures here are 401/403/501 that a retry won't fix.
// Polls (`refetchInterval`) already pause while the tab is hidden: Query's default.
const createQueryClient = () =>
  new QueryClient({
    defaultOptions: {
      queries: { retry: 1, retryDelay, refetchOnWindowFocus: false, staleTime: 15_000 },
      mutations: { retry: 0 },
    },
  });

export { backoff, createQueryClient, MAX_RETRY_DELAY_MS, retryDelay };
