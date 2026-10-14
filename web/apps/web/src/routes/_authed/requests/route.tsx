import { createFileRoute, Outlet } from "@tanstack/react-router";

// Requests (#1405, was Queue) — what you've asked for, and anything that needs you. This file only
// groups the routes: the three tabs share a header and tab bar (`_tabs.tsx`), while a request's
// detail (`$jobId.tsx`) is its own page without them.
const Route = createFileRoute("/_authed/requests")({
  component: Outlet,
});

export { Route };
