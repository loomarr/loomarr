import { createFileRoute } from "@tanstack/react-router";
import { Placeholder } from "@/components/loomarr/feedback/placeholder";

// Authenticated catch-all (§12) — an unknown path inside the app renders dead air,
// still behind the session gate.
const NotFoundScreen = () => <Placeholder title="Off the air" hint="That page doesn't exist." />;

// no-loader: the not-found screen reads nothing
const Route = createFileRoute("/_authed/$")({
  component: NotFoundScreen,
});

export { Route };
