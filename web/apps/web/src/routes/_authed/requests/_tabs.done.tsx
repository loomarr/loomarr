import { createFileRoute } from "@tanstack/react-router";
import { RequestList } from "@/queue/request-lists";

const DoneScreen = () => <RequestList tab="done" />;

// no-loader: reads what the Requests layout's loader ensured
const Route = createFileRoute("/_authed/requests/_tabs/done")({
  component: DoneScreen,
});

export { Route };
