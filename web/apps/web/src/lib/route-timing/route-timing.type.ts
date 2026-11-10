// "rendered": the router committed the page it navigated to. "content": the page says its real
// data is on screen, not a loader or a skeleton standing in for it.
type RoutePhase = "rendered" | "content";

type RouteVisit = {
  id: number;
  // The pathname navigated to.
  to: string;
  // The path was shown earlier in this session, so its data may already be in the cache.
  revisit: boolean;
  // The document's first page: measured from the start of the page load, not from a click.
  documentStart: boolean;
  // When the viewer asked, on the performance clock.
  startedAt: number;
};

type NavigationInfo = {
  fromLocation?: { pathname: string };
  toLocation: { pathname: string };
  pathChanged: boolean;
};

// What the timing module needs from the router: where it is going, and when it got there.
type RouteTimingRouter = {
  subscribe: {
    (event: "onBeforeNavigate", listener: (info: NavigationInfo) => void): () => void;
    (event: "onResolved" | "onRendered", listener: () => void): () => void;
  };
};

export type { NavigationInfo, RoutePhase, RouteTimingRouter, RouteVisit };
