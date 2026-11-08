import type { QueryClient } from "@tanstack/react-query";
import { createRootRouteWithContext, Outlet } from "@tanstack/react-router";
import { RestartOverlay } from "@/components/loomarr/shell/restart-overlay";
import { RestartWatchProvider, useRestartWatchContext } from "@/dashboard/restart-watch-provider";

// The root route carries the TanStack Query client in router context so loaders +
// beforeLoad guards (the _authed gate, login) can ensureQueryData without prop-drilling
// (design §14). It renders the routed Outlet; the app chrome lives in _authed.
//
// The restart watch lives here, above every route: while the server restarts, the _authed guard
// fails and its route's error boundary replaces the shell, and the overlay must outlast that.
const RootLayout = () => (
  <RestartWatchProvider>
    <Outlet />
    <AppRestartOverlay />
  </RestartWatchProvider>
);

const AppRestartOverlay = () => {
  const { restarting, justCameBack, failed } = useRestartWatchContext();
  return <RestartOverlay restarting={restarting} justCameBack={justCameBack} failed={failed} />;
};

const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()({
  component: RootLayout,
});

export { Route };
