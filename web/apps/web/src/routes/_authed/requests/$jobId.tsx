import { createFileRoute } from "@tanstack/react-router";
import { RequestDetail } from "@/queue/request-detail";

const RequestDetailScreen = () => {
  const { jobId } = Route.useParams();
  return <RequestDetail jobId={jobId} />;
};

const Route = createFileRoute("/_authed/requests/$jobId")({
  component: RequestDetailScreen,
});

export { Route };
