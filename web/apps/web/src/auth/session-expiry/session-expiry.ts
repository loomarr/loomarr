import { ApiError } from "@loomarr/api/mutator";
import type { QueryClient } from "@tanstack/react-query";
import { meQueryOptions } from "@/auth/me-query";

const isUnauthorized = (error: unknown) => error instanceof ApiError && error.status === 401;

/**
 * Notices a session that ended while the app was open (signed out elsewhere, revoked, expired) and
 * calls `onSignedOut` once, so the app can return to sign-in instead of failing in place.
 *
 * A 401 alone does not mean the session ended: a wrong current password or a rejected API key also
 * answers 401 to a signed-in viewer. So a 401 only asks `me` again, and the session has ended when
 * `me` answers 401 too. Only a viewer the cache holds as signed in is checked, so the sign-in page's
 * own 401s do nothing. Returns the function that stops watching.
 */
const watchSessionExpiry = (queryClient: QueryClient, onSignedOut: () => void): (() => void) => {
  const me = meQueryOptions();
  let checking = false;

  const check = async () => {
    if (checking || queryClient.getQueryData(me.queryKey)?.status !== 200) return;
    checking = true;
    try {
      await queryClient.fetchQuery({ ...me, staleTime: 0 });
    } catch (error) {
      if (isUnauthorized(error)) {
        queryClient.removeQueries({ queryKey: me.queryKey, exact: true });
        onSignedOut();
      }
    } finally {
      checking = false;
    }
  };

  const onError = (event: { type: string; action?: { type: string; error?: unknown } }) => {
    if (event.type === "updated" && event.action?.type === "error" && isUnauthorized(event.action.error))
      void check();
  };

  const stopQueries = queryClient.getQueryCache().subscribe(onError);
  const stopMutations = queryClient.getMutationCache().subscribe(onError);
  return () => {
    stopQueries();
    stopMutations();
  };
};

export { watchSessionExpiry };
