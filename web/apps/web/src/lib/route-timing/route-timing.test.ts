import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { beginRouteVisit, installRouteTiming, markRoutePhase } from "./route-timing";
import type { NavigationInfo, RouteTimingRouter } from "./route-timing.type";

// A router that emits what the test tells it to, in the order the real one does:
// onBeforeNavigate, then onResolved and onRendered once the page is there.
const fakeRouter = () => {
  const listeners = new Map<string, (info: never) => void>();
  const router = {
    subscribe: (event: string, listener: (info: never) => void) => {
      listeners.set(event, listener);
      return () => listeners.delete(event);
    },
  } as RouteTimingRouter;
  const emit = (event: string, info?: NavigationInfo) => listeners.get(event)?.(info as never);
  return {
    router,
    // `from: null` is the document's first navigation: the router has nowhere it came from.
    navigate: (to: string, from: string | null = "/somewhere") =>
      emit("onBeforeNavigate", {
        fromLocation: from === null ? undefined : { pathname: from },
        toLocation: { pathname: to },
        pathChanged: from !== to,
      }),
    arrive: () => {
      emit("onResolved");
      emit("onRendered");
    },
  };
};

describe("route timing", () => {
  let clock = 0;
  const mark = vi.fn();
  const measure = vi.fn();
  const measured = () => measure.mock.calls.map(([name, options]) => ({ name, detail: options.detail }));
  const requestStart = (visitId: number) =>
    mark.mock.calls.find(([name]) => name === `loomarr:route:${visitId}:request`)?.[1]?.startTime;

  beforeEach(() => {
    clock = 1_000;
    mark.mockReset();
    measure.mockReset();
    vi.stubGlobal("performance", { mark, measure, now: () => clock });
    vi.useFakeTimers({
      toFake: ["setTimeout", "clearTimeout", "requestAnimationFrame", "cancelAnimationFrame"],
    });
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  // Each test starts from a page the router finished, so nothing is waiting from the last one.
  const settle = () => {
    const { router, navigate, arrive } = fakeRouter();
    installRouteTiming(router);
    navigate("/settled");
    arrive();
    vi.runAllTimers();
    mark.mockReset();
    measure.mockReset();
    return { navigate, arrive };
  };

  it("measures a click from the request, under a name that carries no path", () => {
    settle();
    clock = 2_000;
    const visit = beginRouteVisit("/guide-first");
    markRoutePhase(visit.id, "content");

    expect(requestStart(visit.id)).toBe(2_000);
    expect(measured()).toEqual([
      {
        name: "loomarr:route:request-to-content",
        detail: { visitId: visit.id, to: "/guide-first", revisit: false, documentStart: false },
      },
    ]);
  });

  it("starts the document's first page at the start of the page load, through a sign-in redirect too", () => {
    const { router, navigate, arrive } = fakeRouter();
    installRouteTiming(router);

    navigate("/cold-open", null);
    navigate("/sign-in", null); // the guard redirected before anything resolved
    arrive();
    vi.runAllTimers();

    const starts = mark.mock.calls.filter(([name]) => name.endsWith(":request")).map(([, o]) => o.startTime);
    expect(starts).toEqual([0, 0]);
    expect(measured().at(-1)?.detail).toMatchObject({ to: "/sign-in", documentStart: true });
  });

  it("says when a path was shown before in this session", () => {
    const { navigate, arrive } = settle();
    navigate("/library-again");
    arrive();
    vi.runAllTimers();
    navigate("/elsewhere");
    arrive();
    vi.runAllTimers();

    navigate("/library-again");
    arrive();
    vi.runAllTimers();

    expect(measured().map(({ detail }) => `${detail.to} revisit=${detail.revisit}`)).toEqual([
      "/library-again revisit=false",
      "/elsewhere revisit=false",
      "/library-again revisit=true",
    ]);
  });

  it("does not call a path shown before when it was only redirected through", () => {
    const { navigate, arrive } = settle();
    navigate("/guarded"); // sent to sign in before it showed anything
    navigate("/sign-in");
    arrive();
    vi.runAllTimers();

    navigate("/guarded");
    arrive();
    vi.runAllTimers();

    expect(measured().at(-1)?.detail).toMatchObject({ to: "/guarded", revisit: false });
  });

  it("keeps the click's clock through a redirect, because the viewer waited through both", () => {
    const { navigate, arrive } = settle();
    clock = 5_000;
    navigate("/requests");
    clock = 5_040;
    navigate("/requests/in-progress"); // the redirect, before /requests resolved
    arrive();
    vi.runAllTimers();

    const starts = mark.mock.calls.filter(([name]) => name.endsWith(":request")).map(([, o]) => o.startTime);
    expect(starts).toEqual([5_000, 5_000]);
    expect(measured().at(-1)?.detail).toMatchObject({ to: "/requests/in-progress" });
  });

  it("starts a fresh clock once the previous page was reached", () => {
    const { navigate, arrive } = settle();
    clock = 6_000;
    navigate("/first");
    arrive();
    vi.runAllTimers();
    clock = 9_000;
    navigate("/second");

    const starts = mark.mock.calls.filter(([name]) => name.endsWith(":request")).map(([, o]) => o.startTime);
    expect(starts).toEqual([6_000, 9_000]);
  });

  it("counts a phase once, so a refetch on a page already shown does not move its number", () => {
    settle();
    const visit = beginRouteVisit("/channels-once");
    markRoutePhase(visit.id, "content");
    markRoutePhase(visit.id, "content");

    expect(measure).toHaveBeenCalledTimes(1);
  });

  it("ignores a page left mounted behind a newer navigation", () => {
    settle();
    const left = beginRouteVisit("/left-behind");
    beginRouteVisit("/arrived");
    markRoutePhase(left.id, "content");

    expect(measure).not.toHaveBeenCalled();
  });

  it("leaves the page alone when the request mark is no longer in the buffer", () => {
    settle();
    const visit = beginRouteVisit("/buffer-cleared");
    measure.mockImplementation(() => {
      throw new SyntaxError("The request mark has not been set");
    });

    expect(() => markRoutePhase(visit.id, "content")).not.toThrow();
  });

  it("marks a page rendered once it has been painted, not when the router commits it", () => {
    const { navigate, arrive } = settle();
    navigate("/requests-nav");
    arrive();
    expect(measure).not.toHaveBeenCalled();

    vi.runAllTimers();
    expect(measured()).toEqual([
      {
        name: "loomarr:route:request-to-rendered",
        detail: expect.objectContaining({ to: "/requests-nav", documentStart: false }),
      },
    ]);
  });

  it("keeps the visit when only the search or hash changes", () => {
    const { navigate } = settle();

    navigate("/settled", "/settled");

    expect(mark).not.toHaveBeenCalled();
  });
});
