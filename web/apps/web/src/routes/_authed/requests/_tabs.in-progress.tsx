import { createFileRoute } from "@tanstack/react-router";
import { RequestList } from "@/queue/request-lists";

const InProgressScreen = () => <RequestList tab="in-progress" />;

const Route = createFileRoute("/_authed/requests/_tabs/in-progress")({
  component: InProgressScreen,
});

export { Route };
