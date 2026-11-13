import * as systemApi from "@loomarr/api/endpoints/system";
import { Stack } from "@loomarr/design-system";
import { AboutPage as About } from "@loomarr/ui";
import { createFileRoute } from "@tanstack/react-router";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { aboutDetails, aboutVersion } from "@/settings/about-details";

// Settings → System → About (§16, V12): Loomarr's lockup and version, the credits TMDB's licence
// requires (#2151 decision 51), and the bug-report facts behind a disclosure.
//
// ONE query: /v1/system/version carries the runtime, the process start, the backend and the
// applied schema version, so the page cannot render two half-answers from two requests.
const AboutPage = () => {
  const version = systemApi.useSystemVersion();

  if (version.isError) {
    return <ErrorState error={version.error} onRetry={() => void version.refetch()} />;
  }
  if (version.data?.status !== 200) return null;
  const v = version.data.data;

  return (
    <Stack className="h-full min-h-0">
      <PageHeader title="About" description="Loomarr, and the services it's built on." />
      <div className="min-h-0 flex-1 overflow-y-auto p-6">
        <About
          details={aboutDetails(v, Date.now())}
          // TMDB's mark is vendored in public/ (checked for script on the way in), 16px high.
          tmdbLogo={<img alt="TMDB" src="/tmdb.svg" style={{ height: 16, width: "auto" }} />}
          version={aboutVersion(v)}
        />
      </div>
    </Stack>
  );
};

const Route = createFileRoute("/_authed/settings/system/about")({
  component: AboutPage,
  // The one query the page is made of. The shell reads it too, so this is usually already cached.
  loader: async ({ context: { queryClient } }) => {
    await queryClient.ensureQueryData(systemApi.getSystemVersionQueryOptions());
  },
});

export { Route };
