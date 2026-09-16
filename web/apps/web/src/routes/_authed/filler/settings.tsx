import { createFileRoute } from "@tanstack/react-router";
import { FillerPage } from "@/filler/filler-page";
import { validateFillerSettingsSearch } from "@/filler/filler-settings/filler-settings-search";
import { SettingsEditsProvider } from "@/settings/settings-edits";
import { SettingsSaveBarHost } from "@/settings/settings-save-bar-host";

const FillerSettingsScreen = () => (
  <SettingsEditsProvider>
    <div className="flex h-full min-h-0 flex-col">
      <div className="min-h-0 flex-1">
        <FillerPage tab="settings" />
      </div>
      <SettingsSaveBarHost />
    </div>
  </SettingsEditsProvider>
);

const Route = createFileRoute("/_authed/filler/settings")({
  validateSearch: validateFillerSettingsSearch,
  component: FillerSettingsScreen,
});

export { Route };
