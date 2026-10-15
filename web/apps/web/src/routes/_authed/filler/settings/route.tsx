import { createFileRoute, Outlet } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { SettingsEditsProvider } from "@/settings/settings-edits";
import { SettingsSaveBarHost } from "@/settings/settings-save-bar-host";

// Admin-only: a member gets AdminOnly's no-access state, and nothing below it (with its private
// settings read) mounts.
const FillerSettingsLayout = () => (
  <AdminOnly>
    <SettingsEditsProvider>
      <div className="flex h-full min-h-0 flex-col">
        <div className="min-h-0 flex-1">
          <Outlet />
        </div>
        <SettingsSaveBarHost />
      </div>
    </SettingsEditsProvider>
  </AdminOnly>
);

const Route = createFileRoute("/_authed/filler/settings")({
  component: FillerSettingsLayout,
});

export { Route };
