import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
  useRouter,
} from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { beginRouteVisit, installRouteTiming } from "@/lib/route-timing";

import { useRouteContent } from "./use-route-content";

const Page = ({ ready }: { ready: boolean }) => {
  useRouteContent(ready);
  return <p>{ready ? "the guide" : "loading"}</p>;
};

describe("useRouteContent", () => {
  const measure = vi.fn();
  const contentMeasures = () =>
    measure.mock.calls
      .filter(([name]) => name === "loomarr:route:request-to-content")
      .map(([, o]) => o.detail.to);

  beforeEach(() => {
    measure.mockReset();
    vi.stubGlobal("performance", { mark: vi.fn(), measure, now: () => 0 });
    vi.useFakeTimers({
      toFake: ["setTimeout", "clearTimeout", "requestAnimationFrame", "cancelAnimationFrame"],
    });
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("times the visit once the page's content has been painted, not while it loads", () => {
    beginRouteVisit("/hook-guide");
    const page = render(<Page ready={false} />);
    vi.runAllTimers();
    expect(contentMeasures()).toEqual([]);

    page.rerender(<Page ready />);
    expect(contentMeasures()).toEqual([]); // committed, not yet painted
    vi.runAllTimers();
    expect(contentMeasures()).toEqual(["/hook-guide"]);
  });

  it("reports nothing for the next page from a page that is still mounted behind it", () => {
    beginRouteVisit("/hook-old");
    const old = render(<Page ready={false} />);
    beginRouteVisit("/hook-new");

    old.rerender(<Page ready />);
    vi.runAllTimers();

    expect(contentMeasures()).toEqual([]);
  });

  it("does not report content the page stopped showing before it was painted", () => {
    beginRouteVisit("/hook-flicker");
    const page = render(<Page ready />);
    page.rerender(<Page ready={false} />);
    vi.runAllTimers();

    expect(contentMeasures()).toEqual([]);
  });
});

// The same hook under the real router, on real timers: this is what proves the router's own
// events start and finish a visit the way the timing module expects.
describe("route timing through the router", () => {
  // Read back from the real User Timing buffer, the way a perf test in a browser does.
  const measured = (phase: "rendered" | "content") =>
    performance.getEntriesByName(`loomarr:route:request-to-${phase}`, "measure").map((entry) => {
      const { to, documentStart } = (entry as PerformanceMeasure).detail;
      return { to, documentStart };
    });

  beforeEach(() => {
    performance.clearMarks();
    performance.clearMeasures();
  });

  // The test's two paths are not in the app's route tree, so it navigates through the history
  // rather than a typed Link.
  const StartPage = () => {
    useRouteContent(true);
    const { history } = useRouter();
    return (
      <button type="button" onClick={() => history.push("/timed-rows")}>
        Open the rows
      </button>
    );
  };
  // Shows a loader first, as a page that fetches after it mounts does.
  const RowsPage = () => {
    const [rows, setRows] = useState<string>();
    useEffect(() => {
      const timer = setTimeout(() => setRows("Three rows"), 30);
      return () => clearTimeout(timer);
    }, []);
    useRouteContent(rows !== undefined);
    return <p>{rows ?? "Loading rows"}</p>;
  };

  it("times the first page from the page load and a click from the click, each to its content", async () => {
    const rootRoute = createRootRoute({ component: Outlet });
    const router = createRouter({
      history: createMemoryHistory({ initialEntries: ["/timed-start"] }),
      routeTree: rootRoute.addChildren([
        createRoute({ getParentRoute: () => rootRoute, path: "/timed-start", component: StartPage }),
        createRoute({ getParentRoute: () => rootRoute, path: "/timed-rows", component: RowsPage }),
      ]),
    });
    const stop = installRouteTiming(router);
    render(<RouterProvider router={router} />);

    await waitFor(() => expect(measured("content")).toEqual([{ to: "/timed-start", documentStart: true }]));

    await userEvent.click(await screen.findByRole("button", { name: "Open the rows" }));
    expect(await screen.findByText("Loading rows")).toBeInTheDocument();
    await waitFor(() =>
      expect(measured("rendered")).toContainEqual({ to: "/timed-rows", documentStart: false }),
    );
    // Rendered is not content: the loader was on screen when the router finished.
    expect(measured("content")).toHaveLength(1);

    expect(await screen.findByText("Three rows")).toBeInTheDocument();
    await waitFor(() =>
      expect(measured("content").at(-1)).toEqual({ to: "/timed-rows", documentStart: false }),
    );
    stop();
  });
});
