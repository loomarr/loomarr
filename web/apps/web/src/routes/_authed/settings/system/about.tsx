import { systemApi } from "@loomarr/api";
import { createFileRoute } from "@tanstack/react-router";
import { AboutPanel, ErrorState } from "@/components/loomarr";

// Settings → System → About (§16, V12) — what the operator quotes in a bug report.
//
// The database backend comes from the database-status endpoint rather than the version
// one. It is rendered when available and omitted when not: About must not fail to load
// because a second, unrelated query did.
const AboutPage = () => {
  const version = systemApi.useSystemVersion();
  const database = systemApi.useSystemDatabaseStatus();

  if (version.isError) {
    return <ErrorState error={version.error} onRetry={() => void version.refetch()} />;
  }
  if (!version.data || version.data.status !== 200) return null;

  const backend = database.data?.status === 200 ? database.data.data.backend : undefined;

  return (
    <div className="overflow-y-auto p-6">
      <AboutPanel version={version.data.data} backend={backend} />
    </div>
  );
};

const Route = createFileRoute("/_authed/settings/system/about")({
  component: AboutPage,
});

export { Route };
