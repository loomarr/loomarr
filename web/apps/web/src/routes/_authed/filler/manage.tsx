import { createFileRoute } from "@tanstack/react-router";
import { FillerPage } from "@/filler/filler-page";

const ManageScreen = () => <FillerPage tab="manage" />;

const Route = createFileRoute("/_authed/filler/manage")({
  component: ManageScreen,
});

export { Route };
