import { createFileRoute } from "@tanstack/react-router";
import { FillerPage } from "@/filler/filler-page";

const AdvancedScreen = () => <FillerPage tab="advanced" />;

const Route = createFileRoute("/_authed/filler/advanced")({
  component: AdvancedScreen,
});

export { Route };
