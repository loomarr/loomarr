import * as settingsApi from "@loomarr/api/endpoints/settings";
import { createFileRoute, Outlet, useLocation } from "@tanstack/react-router";
import { useAuth } from "@/auth/use-auth";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useDocumentTitle } from "@/lib/use-document-title";
import { SettingsContextNav } from "@/settings/settings-context-nav";
import { SettingsEditsProvider } from "@/settings/settings-edits";
import { SettingsSaveBarHost } from "@/settings/settings-save-bar-host";

const SettingsLayout = () => {
  useDocumentTitle("Settings");
  const { isAdmin } = useAuth();
  // ⚠ Read BEFORE the error early-return below: a hook after a conditional return is skipped on
  // that path, which React reports as "rendered fewer hooks than expected" and replaces the page
  // with an error boundary. (The same trap `filler-page.tsx` records hitting.)
  const { pathname } = useLocation();
  const memberNotifications = !isAdmin && pathname.startsWith("/settings/notifications");
  const settings = settingsApi.useSettingsList({ query: { enabled: isAdmin, retry: false } });

  if (settings.error && !memberNotifications) {
    return (
      <div className="p-6">
        <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
      </div>
    );
  }

  const normalizedPath = pathname.replace(/\/+$/, "");
  const showContextNav = normalizedPath !== "/settings";

  return (
    // ⚠ The provider wraps the OUTLET, which is what makes the save bar cross-tab: the buffer
    // belongs to the layout, so switching pages re-renders the outlet without unmounting the
    // edits. Held one level lower (inside a page) it died on every tab switch.
    <SettingsEditsProvider>
      <div className="flex h-full flex-col">
        {showContextNav ? (
          <SettingsContextNav section={pathname.startsWith("/settings/system") ? "This server" : undefined} />
        ) : null}
        <div className="min-w-0 flex-1 overflow-hidden">
          <Outlet />
        </div>
        <SettingsSaveBarHost />
      </div>
    </SettingsEditsProvider>
  );
};

const Route = createFileRoute("/_authed/settings")({
  component: SettingsLayout,
});

export { Route };
