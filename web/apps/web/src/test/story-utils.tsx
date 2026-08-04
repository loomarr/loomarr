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
//
// ⚠ `/wizard` is here because ChecklistItem's stories start there. It was missing, so the
// harness matched no route, rendered TanStack's "Not Found", and the visual suite snapshotted
// THAT — all four ChecklistItem states (× both viewports) were one identical 1160-byte PNG of
// the words "Not Found", asserted as the baseline on every run. See the guard below.
const NAV_PATHS = [
  "/dashboard",
  "/guide",
  "/queue",
  "/filler",
  "/people",
  "/settings",
  "/help",
  "/wizard",
] as const;

// Mounts `content` inside a minimal in-memory TanStack Router covering the AppShell nav,
// starting at `initialPath`. Anything using Link/route hooks then renders the same
// offline + deterministic way it would inside the real app router — TanStack `Link`
// needs a RouterProvider even in isolation (the migration's one harness cost).
const RouterHarness = ({ content, initialPath = "/guide" }: { content: ReactNode; initialPath?: string }) => {
  // ⚠ THROW on an unregistered path rather than letting the router render "Not Found".
  //
  // A silent miss here is invisible in exactly the place it matters most: the story still
  // mounts something, `#storybook-root > *` becomes visible, the play-function phase reaches
  // "finished", and the visual suite screenshots a "Not Found" page and commits it as the
  // baseline. It then passes forever, because the page is perfectly stable. That is how four
  // ChecklistItem stories spent releases asserting a 1160-byte error page while appearing to
  // cover pass/fail/pending/running.
  //
  // Throwing turns a silent wrong-baseline into a loud failure at the one moment someone is
  // looking — when they add the story.
  const base = initialPath.split("?")[0]?.split("#")[0] ?? initialPath;
  if (!NAV_PATHS.some((path) => base === path || base.startsWith(`${path}/`))) {
    throw new Error(
      `RouterHarness: "${initialPath}" is not a registered route, so this would render "Not Found" ` +
        `instead of the component. Add it to NAV_PATHS in story-utils.tsx. Registered: ${NAV_PATHS.join(", ")}`,
    );
  }
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
