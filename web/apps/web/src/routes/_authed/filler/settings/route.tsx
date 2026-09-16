import { createFileRoute, Outlet } from "@tanstack/react-router";
import { SettingsEditsProvider } from "@/settings/settings-edits";
import { SettingsSaveBarHost } from "@/settings/settings-save-bar-host";

const FillerSettingsLayout = () => (
  <SettingsEditsProvider>
    <div className="flex h-full min-h-0 flex-col">
      <div className="min-h-0 flex-1">
        <Outlet />
      </div>
      <SettingsSaveBarHost />
    </div>
  </SettingsEditsProvider>
);

const Route = createFileRoute("/_authed/filler/settings")({
  component: FillerSettingsLayout,
});

export { Route };
