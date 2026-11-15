import { Stack } from "@loomarr/design-system";
import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { meQueryOptions } from "@/auth/me-query";
import { SettingsEditsProvider } from "@/settings/settings-edits";
import { SettingsSaveBarHost } from "@/settings/settings-save-bar-host";

const FillerSettingsLayout = () => (
  <SettingsEditsProvider>
    <Stack height="100%" minHeight={0}>
      <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minHeight={0}>
        <Outlet />
      </Stack>
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
