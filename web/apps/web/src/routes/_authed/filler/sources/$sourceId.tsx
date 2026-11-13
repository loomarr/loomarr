import { createFileRoute } from "@tanstack/react-router";

// no-loader: renders nothing of its own; the Sources layout above holds the page and its data
const Route = createFileRoute("/_authed/filler/sources/$sourceId")({});

export { Route };
