import type { Decorator } from "@storybook/react-vite";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import type { ReactNode } from "react";

// A fixed-width frame so `w-full` components (IntentInput, ProposalReview, SearchCommand,
// …) render at a realistic size inside the centered story canvas — the snapshot then
// bounds the component the way it sits in a real page column, not collapsed to zero.
const widthFrame =
  (px: number): Decorator =>
  (Story) => (
    <div style={{ width: px, maxWidth: "100%" }}>
      <Story />
    </div>
  );

// AppShell links to these routes; any router harness must register them so TanStack
// `Link` can resolve each href when the component renders in isolation (stories + units).
const NAV_PATHS = ["/dashboard", "/guide", "/queue", "/filler", "/people", "/settings", "/help"] as const;

// Mounts `content` inside a minimal in-memory TanStack Router covering the AppShell nav,
// starting at `initialPath`. Anything using Link/route hooks then renders the same
// offline + deterministic way it would inside the real app router — TanStack `Link`
// needs a RouterProvider even in isolation (the migration's one harness cost).
const RouterHarness = ({ content, initialPath = "/guide" }: { content: ReactNode; initialPath?: string }) => {
  const rootRoute = createRootRoute();
  const routes = NAV_PATHS.map((path) =>
    createRoute({ getParentRoute: () => rootRoute, path, component: () => <>{content}</> }),
  );
  const router = createRouter({
    routeTree: rootRoute.addChildren(routes),
    history: createMemoryHistory({ initialEntries: [initialPath] }),
  });
  return <RouterProvider router={router} />;
};

const withRouter =
  (initialPath = "/guide"): Decorator =>
  (Story) => <RouterHarness content={<Story />} initialPath={initialPath} />;

export { RouterHarness, widthFrame, withRouter };
