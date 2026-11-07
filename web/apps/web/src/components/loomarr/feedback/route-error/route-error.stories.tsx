import { ApiError } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { widthFrame } from "@/test/story-utils";
import { RouteError } from "./route-error";

// The router's default error state: what a route shows when its guard or loader fails. RouteError
// needs a router (Try again invalidates it), so the story mounts one whose
// only route always fails.
const FailingRoute = () => {
  const rootRoute = createRootRoute({
    beforeLoad: () => {
      throw new ApiError(503, {
        title: "Service Unavailable",
        detail: "Loomarr is restarting.",
        status: 503,
      });
    },
  });
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory(),
    defaultErrorComponent: RouteError,
  });
  return <RouterProvider router={router} />;
};

const meta = {
  title: "Feedback/RouteError",
  component: FailingRoute,
  decorators: [widthFrame(640)],
} satisfies Meta<typeof FailingRoute>;

type Story = StoryObj<typeof meta>;

const Failed: Story = {};

export default meta;
export { Failed };
