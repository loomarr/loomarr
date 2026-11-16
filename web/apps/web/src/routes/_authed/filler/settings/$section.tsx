import * as fillerApi from "@loomarr/api/endpoints/filler";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { warmFillerPage } from "@/filler/filler-loader";
import { FillerPage } from "@/filler/filler-page";
import { parseFillerSettingsSection } from "@/filler/filler-settings-section";

const FillerSettingsScreen = () => {
  const { section: rawSection } = Route.useParams();
  const section = parseFillerSettingsSection(rawSection) ?? "downloads";
  return <FillerPage tab="settings" settingsSection={section} />;
};

const Route = createFileRoute("/_authed/filler/settings/$section")({
  beforeLoad: ({ params }) => {
    if (!parseFillerSettingsSection(params.section)) {
      throw redirect({ to: "/filler/settings" });
    }
  },
  // The settings list the fields read (warmed with the page's status line), the providers the
  // downloads section lists, and the one read each of Downloads and Storage adds. Admin-only: the
  // layout's beforeLoad has already sent a member away. A section that fails answers for itself.
  loader: async ({ context: { queryClient }, params }) => {
    const section = parseFillerSettingsSection(params.section);
    await Promise.all([
      warmFillerPage(queryClient),
      queryClient.prefetchQuery(fillerApi.getListFillerProvidersQueryOptions()),
      section === "downloads"
        ? queryClient.prefetchQuery(fillerApi.getListFillerSourcesQueryOptions())
        : undefined,
      section === "storage"
        ? queryClient.prefetchQuery(fillerApi.getFillerReadinessQueryOptions())
        : undefined,
    ]);
  },
  component: FillerSettingsScreen,
});

export { Route };
