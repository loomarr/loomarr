import { createFileRoute } from "@tanstack/react-router";
import { RequestList } from "@/queue/request-lists";

const DoneScreen = () => <RequestList tab="done" />;

const Route = createFileRoute("/_authed/requests/_tabs/done")({
  component: DoneScreen,
});

export { Route };
