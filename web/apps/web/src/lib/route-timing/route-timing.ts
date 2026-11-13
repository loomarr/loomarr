import type { RoutePhase, RouteTimingRouter, RouteVisit } from "./route-timing.type";

// Route timing answers one question in the browser's own clock: how long after asking for a page
// did its content appear (#3399)? It writes User Timing entries, the same instrument the tuner
// uses, so a perf test and the browser's Performance panel read the same numbers.
//
//   loomarr:route:request-to-rendered   the router committed the page
//   loomarr:route:request-to-content    the page's real data is painted
//
// The names carry no path. Each measure's detail says which path it was, whether the path had
// been shown before in this session, and whether it was the document's first page.

let nextVisitId = 1;
let current: RouteVisit | undefined;
// The router finished the navigation in progress: it was not redirected away or abandoned.
let resolved = false;
let reported = new Set<RoutePhase>();
const shown = new Set<string>();

const canMeasure = () =>
  typeof performance?.mark === "function" && typeof performance?.measure === "function";

const markName = (visit: RouteVisit, phase: "request" | RoutePhase) => `loomarr:route:${visit.id}:${phase}`;

/**
 * A page was asked for. `documentStart` is the page the document opened on: its clock starts
 * when the page load did, because no click preceded it.
 *
 * A visit the router never resolved was a redirect, or a click the viewer did not wait for.
 * The viewer has been waiting since it began, so the visit that replaces it keeps its clock:
 * `/requests` redirecting to `/requests/in-progress` is one wait, measured from the click.
 */
const beginRouteVisit = (to: string, documentStart = false): RouteVisit => {
  const waiting = current && !resolved ? current : undefined;
  const fromDocumentStart = waiting?.documentStart ?? documentStart;
  const visit = {
    id: nextVisitId++,
    to,
    revisit: shown.has(to),
    documentStart: fromDocumentStart,
    startedAt: waiting?.startedAt ?? (fromDocumentStart ? 0 : performance.now()),
  };
  current = visit;
  resolved = false;
  reported = new Set();
  if (canMeasure()) performance.mark(markName(visit, "request"), { startTime: visit.startedAt });
  return visit;
};

/** The id of the visit in progress. A page reads it as it mounts, to report against later. */
const currentRouteVisit = (): number | undefined => current?.id;

/**
 * Records that the visit reached a phase. Only the visit in progress counts, and only the first
 * time: a page left mounted behind a newer navigation holds an older id and reports nothing, and
 * a refetch on a page already shown does not move its number.
 */
const markRoutePhase = (visitId: number | undefined, phase: RoutePhase) => {
  if (!current || current.id !== visitId || reported.has(phase) || !canMeasure()) return;
  reported.add(phase);
  shown.add(current.to);
  const start = markName(current, "request");
  const end = markName(current, phase);
  performance.mark(end);
  try {
    performance.measure(`loomarr:route:request-to-${phase}`, {
      start,
      end,
      detail: {
        visitId: current.id,
        to: current.to,
        revisit: current.revisit,
        documentStart: current.documentStart,
      },
    });
  } catch {
    // The request mark is gone (something cleared the buffer). A missing number is the right
    // outcome: timing a page must never be what breaks it.
  }
};

/**
 * Runs `callback` once the browser has painted the frame that follows this call, so a phase is
 * timed when the viewer could see it, not when React committed it. Returns a cancel.
 */
const afterNextPaint = (callback: () => void): (() => void) => {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const frame = requestAnimationFrame(() => {
    timer = setTimeout(callback, 0);
  });
  return () => {
    cancelAnimationFrame(frame);
    if (timer !== undefined) clearTimeout(timer);
  };
};

/**
 * Starts a visit on every navigation to another path and marks it rendered once painted.
 * Installing begins a clean session: no visit is waiting and no path counts as shown.
 */
const installRouteTiming = (router: RouteTimingRouter): (() => void) => {
  current = undefined;
  resolved = false;
  reported = new Set();
  shown.clear();
  const stops = [
    router.subscribe("onBeforeNavigate", ({ fromLocation, toLocation, pathChanged }) => {
      if (fromLocation && !pathChanged) return; // a search or hash change keeps the page
      beginRouteVisit(toLocation.pathname, !fromLocation);
    }),
    router.subscribe("onResolved", () => {
      resolved = true;
    }),
    router.subscribe("onRendered", () => {
      const visitId = currentRouteVisit();
      afterNextPaint(() => markRoutePhase(visitId, "rendered"));
    }),
  ];
  return () => {
    for (const stop of stops) stop();
  };
};

export { afterNextPaint, beginRouteVisit, currentRouteVisit, installRouteTiming, markRoutePhase };
