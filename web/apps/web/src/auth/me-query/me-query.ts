import * as authApi from "@loomarr/api/endpoints/auth";
import type { QueryClient } from "@tanstack/react-query";

// The single definition of the GET /v1/auth/me query, shared by useAuth (render-time
// identity) and the router beforeLoad guards (ensureQueryData in _authed/login).
// retry:false — a 401 is a definite answer (signed out), not a transient failure (§11).
const meQueryOptions = () => authApi.getMeQueryOptions({ query: { retry: false } });

// Whether the viewer is an admin, for a route's `beforeLoad` or `loader`. The `_authed` layout has
// already primed `me`, so this reads the cache; it is how a loader skips the admin-only reads a
// member's request would 403 on, exactly as the page's own `enabled: isAdmin` does.
const viewerIsAdmin = async (queryClient: QueryClient): Promise<boolean> => {
  const me = await queryClient.ensureQueryData(meQueryOptions());
  return me.status === 200 && me.data.role === "admin";
};

export { meQueryOptions, viewerIsAdmin };
