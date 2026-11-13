import { createFileRoute } from "@tanstack/react-router";
import { RequestList } from "@/queue/request-lists";

const InProgressScreen = () => <RequestList tab="in-progress" />;

// no-loader: reads what the Requests layout's loader ensured
const Route = createFileRoute("/_authed/requests/_tabs/in-progress")({
  component: InProgressScreen,
});

export { Route };
