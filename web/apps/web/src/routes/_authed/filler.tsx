import { createFileRoute } from "@tanstack/react-router";
import { FillerPage } from "@/filler";

// Browsing and previewing filler is readable by any authenticated user (it explains why a
// channel plays what it plays); the mutating actions inside are admin-gated, and the
// server enforces that regardless (§11, §19).
const FillerScreen = () => <FillerPage />;

const Route = createFileRoute("/_authed/filler")({
  component: FillerScreen,
});

export { Route };
