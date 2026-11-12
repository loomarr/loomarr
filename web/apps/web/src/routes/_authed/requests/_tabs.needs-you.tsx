import { createFileRoute } from "@tanstack/react-router";
import { NeedsYouList } from "@/queue/request-lists";

const NeedsYouScreen = () => <NeedsYouList />;

// no-loader: reads what the Requests layout's loader ensured
const Route = createFileRoute("/_authed/requests/_tabs/needs-you")({
  component: NeedsYouScreen,
});

export { Route };
