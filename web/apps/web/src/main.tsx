import { LoomarrProvider } from "@loomarr/design-system";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createRouter, type ErrorComponentProps, RouterProvider } from "@tanstack/react-router";
import { lazy, StrictMode, Suspense } from "react";
import { createRoot } from "react-dom/client";
import { Toaster } from "sonner";
import { clientDiagnostics, installGlobalClientDiagnostics } from "@/diagnostics/client-reporter";
import { routeTree } from "@/routeTree.gen";
// Self-hosted Geist (§2.2) — bundled by Vite, no CDN, deterministic visual tests.
import "@fontsource-variable/geist";
import "@fontsource-variable/geist-mono";
import "@/styles.css";

installGlobalClientDiagnostics();

// Server state is TanStack Query, SSE-invalidated (no global store, §4.3). Retries
// are conservative — most failures here are 401/403/501 that a retry won't fix.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false, staleTime: 15_000 },
    mutations: { retry: 0 },
  },
});

// The router shares the Query client via context so route guards (the _authed gate,
// login) can ensureQueryData without prop-drilling (design §14). Register makes every
// Link/navigate across the app type-safe against this route tree.
// `defaultPreload: "intent"` runs a route's loader on HOVER (and focus), so a nav click lands on
// data that is already in flight or already there. The Guide is what motivated it: its request is
// the app's most expensive read, and the difference between "spinner then rows" and "rows" is the
// pointer travel time that was being wasted anyway.
//
// preloadStaleTime pairs with it: without one, TanStack treats preloaded data as immediately
// stale and the component refetches on mount — paying the request twice and defeating the point.
// 15s matches the query defaults above, so a preload is trusted exactly as long as any other
// cached read.
//
// The route error and pending views are design-system views, and everything above the routes is
// in the first download. Loaded on first use, they keep the design-system runtime (about 380 KiB)
// out of it: every route is code-split, so a page that renders brings that runtime with it anyway.
const LazyRouteError = lazy(() =>
  import("@/components/loomarr/feedback/route-error").then(({ RouteError }) => ({ default: RouteError })),
);
const LazyRoutePending = lazy(() =>
  import("@/components/loomarr/feedback/route-pending").then(({ RoutePending }) => ({
    default: RoutePending,
  })),
);

const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultPreload: "intent",
  defaultPreloadStaleTime: 15_000,
  defaultErrorComponent: (props: ErrorComponentProps) => (
    <Suspense fallback={null}>
      <LazyRouteError {...props} />
    </Suspense>
  ),
  defaultPendingComponent: () => (
    <Suspense fallback={null}>
      <LazyRoutePending />
    </Suspense>
  ),
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

createRoot(document.getElementById("root")!, {
  onCaughtError: () =>
    clientDiagnostics.record({ event: "client.error_boundary", surface: "root", errorClass: "react_error" }),
  onUncaughtError: () =>
    clientDiagnostics.record({ event: "client.unhandled_error", surface: "root", errorClass: "react_error" }),
}).render(
  <StrictMode>
    <LoomarrProvider theme="dark">
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
        <Toaster
          theme="dark"
          position="bottom-center"
          offset={{ bottom: 84 }}
          mobileOffset={{ bottom: 84 }}
          richColors
        />
      </QueryClientProvider>
    </LoomarrProvider>
  </StrictMode>,
);
