import type { QueryClient } from "@tanstack/react-query";
import { createRootRouteWithContext, Outlet } from "@tanstack/react-router";

// The root route carries the TanStack Query client in router context so loaders +
// beforeLoad guards (the _authed gate, login) can ensureQueryData without prop-drilling
// (design §14). It renders only the routed Outlet — the app chrome lives in _authed.
const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()({
  component: Outlet,
});

export { Route };
