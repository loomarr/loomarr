import { createFileRoute } from "@tanstack/react-router";
import { FillerPage } from "@/filler/filler-page";

const AttentionScreen = () => <FillerPage tab="attention" />;

const Route = createFileRoute("/_authed/filler/attention")({
  component: AttentionScreen,
});

export { Route };
