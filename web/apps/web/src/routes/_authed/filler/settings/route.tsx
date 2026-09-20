import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { meQueryOptions } from "@/auth/me-query";
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
  beforeLoad: async ({ context }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions());
    if (me.status !== 200 || me.data.role !== "admin") {
      throw redirect({ to: "/filler/manage" });
    }
  },
  component: FillerSettingsLayout,
});

export { Route };
