import type { QueryClient } from "@tanstack/react-query";
import { createRootRouteWithContext, Outlet } from "@tanstack/react-router";
import { TooltipProvider } from "@/components/ui/tooltip";

// The root route carries the TanStack Query client in router context so loaders +
// beforeLoad guards (the _authed gate, login) can ensureQueryData without prop-drilling
// (design §14). It renders the routed Outlet — the app chrome lives in _authed — inside a
// single TooltipProvider so every route (login, wizard, app) can label its icon-only
// affordances. The delay is snappy (300ms) so a hover reveals the label promptly.
const RootLayout = () => (
  <TooltipProvider delay={300}>
    <Outlet />
  </TooltipProvider>
);

const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()({
  component: RootLayout,
});

export { Route };
