import * as systemApi from "@loomarr/api/endpoints/system";
import { createFileRoute } from "@tanstack/react-router";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { AboutPanel } from "@/components/loomarr/settings/about-panel";

// Settings → System → About (§16, V12) — what the operator quotes in a bug report.
//
// ONE query. The backend used to come from the database-status endpoint while everything
// else came from the version endpoint; now /v1/system/version carries the runtime, the
// process start, the backend and the applied schema version, so the page has a single
// source and cannot render two half-answers from two requests that resolved differently.
const AboutPage = () => {
  const version = systemApi.useSystemVersion();

  if (version.isError) {
    return <ErrorState error={version.error} onRetry={() => void version.refetch()} />;
  }
  if (version.data?.status !== 200) return null;

  return (
    <div className="overflow-y-auto p-6">
      <AboutPanel version={version.data.data} />
    </div>
  );
};

const Route = createFileRoute("/_authed/settings/system/about")({
  component: AboutPage,
});

export { Route };
