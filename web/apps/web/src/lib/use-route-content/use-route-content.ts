import { useEffect, useRef } from "react";

import { afterNextPaint, currentRouteVisit, markRoutePhase } from "@/lib/route-timing";

/**
 * A page calls this with whether its real content is on screen: the data it exists to show,
 * not a loader or a skeleton in its place. The first time that is true, once painted, the visit
 * that mounted the page is timed as `loomarr:route:request-to-content` (#3399).
 *
 * The visit is read at mount, so a page still mounted behind a newer navigation cannot report
 * content for the page that is replacing it.
 */
const useRouteContent = (ready: boolean) => {
  const visit = useRef(currentRouteVisit());
  useEffect(() => {
    if (!ready) return undefined;
    return afterNextPaint(() => markRoutePhase(visit.current, "content"));
  }, [ready]);
};

export { useRouteContent };
