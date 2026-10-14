import { createFileRoute, redirect } from "@tanstack/react-router";
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
  component: FillerSettingsScreen,
});

export { Route };
