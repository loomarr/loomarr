import { Stack } from "@loomarr/design-system";
import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { meQueryOptions } from "@/auth/me-query";
import { SettingsEditsProvider } from "@/settings/settings-edits";
import { SettingsSaveBarHost } from "@/settings/settings-save-bar-host";

const FillerSettingsLayout = () => (
  <SettingsEditsProvider>
    <Stack className="h-full min-h-0">
      <div className="min-h-0 flex-1">
        <Outlet />
      </div>
      <SettingsSaveBarHost />
    </Stack>
  </SettingsEditsProvider>
);

// no-loader: only gates the Settings pages to admins and wraps their outlet
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
