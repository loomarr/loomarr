import { ApiError } from "@loomarr/api";
import { LoomarrProvider } from "@loomarr/design-system";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { RouteError } from "./route-error";

// A route whose guard fails until `healthy` flips, rendered through the real router so Try again
// exercises reset + invalidate, not a stubbed callback.
const renderFailingRoute = () => {
  const state = { healthy: false };
  const rootRoute = createRootRoute({
    beforeLoad: () => {
      if (!state.healthy) {
        throw new ApiError(503, {
          title: "Service Unavailable",
          detail: "Loomarr is restarting.",
          status: 503,
        });
      }
    },
    component: () => <p>Back on air</p>,
  });
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory(),
    defaultErrorComponent: RouteError,
  });
  render(
    <LoomarrProvider theme="dark">
      <RouterProvider router={router} />
    </LoomarrProvider>,
  );
  return state;
};

describe("RouteError", () => {
  it("says what failed in the problem's own words and retries the route", async () => {
    const state = renderFailingRoute();
    expect(await screen.findByText("Couldn't load this page")).toBeInTheDocument();
    expect(screen.getByText("Loomarr is restarting.")).toBeInTheDocument();

    state.healthy = true;
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("Back on air")).toBeInTheDocument();
  });
});
