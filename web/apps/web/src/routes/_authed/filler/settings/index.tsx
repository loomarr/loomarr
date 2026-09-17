import { createFileRoute } from "@tanstack/react-router";
import { FillerPage } from "@/filler/filler-page";

const Route = createFileRoute("/_authed/filler/settings/")({
  component: () => <FillerPage tab="settings" />,
});

export { Route };
