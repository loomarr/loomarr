import * as settingsApi from "@loomarr/api/endpoints/settings";
import { createFileRoute, Outlet, useChildMatches, useLocation } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { useAuth } from "@/auth/use-auth";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useDocumentTitle } from "@/lib/use-document-title";
import { SettingsContextNav } from "@/settings/settings-context-nav";
import { SETTINGS_DESTINATIONS } from "@/settings/settings-destinations";
import { SettingsEditsProvider } from "@/settings/settings-edits";
import { SettingsSaveBarHost } from "@/settings/settings-save-bar-host";

// What a member may open under /settings: the index (it lists only what they can visit) and the
// destinations the registry marks for everyone. Everything else is an admin page (#1977).
const memberCanOpen = (path: string): boolean =>
  path === "/settings" ||
  SETTINGS_DESTINATIONS.some(
    (item) => item.role === "all" && (path === item.path || path.startsWith(`${item.path}/`)),
  );

const SettingsLayout = () => {
  useDocumentTitle("Settings");
  const { isAdmin } = useAuth();
  // ⚠ Read BEFORE the error early-return below: a hook after a conditional return is skipped on
  // that path, which React reports as "rendered fewer hooks than expected" and replaces the page
  // with an error boundary. (The same trap `filler-page.tsx` records hitting.)
  const { pathname } = useLocation();
  const childPath = useChildMatches({ select: (matches) => matches.at(-1)?.pathname });
  const settings = settingsApi.useSettingsList({ query: { enabled: isAdmin, retry: false } });

  // ⚠ Admins only. The list 403s for a member, and pages a member CAN open (the index) read the
  // same query key through useSettingsEntries, so its error lands here too. Shown to a member,
  // it replaced their own Settings with a bare "Forbidden" (#1977).
  if (settings.error && isAdmin) {
    return (
      <div className="p-6">
        <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
      </div>
    );
  }

  const normalizedPath = pathname.replace(/\/+$/, "");
  const showContextNav = normalizedPath !== "/settings";
  // ⚠ The RENDERED page decides the gate, not the location. On a navigation away, useLocation moves
  // to the new address while the old page is still in the Outlet, so a member leaving
  // Notifications would flash "Admins only", and the reverse would mount an admin page bare.
  const renderedPath = (childPath ?? normalizedPath).replace(/\/+$/, "");
  const page = memberCanOpen(renderedPath) ? (
    <Outlet />
  ) : (
    <AdminOnly>
      <Outlet />
    </AdminOnly>
  );

  return (
    // ⚠ The provider wraps the OUTLET, which is what makes the save bar cross-tab: the buffer
    // belongs to the layout, so switching pages re-renders the outlet without unmounting the
    // edits. Held one level lower (inside a page) it died on every tab switch.
    <SettingsEditsProvider>
      <div className="flex h-full flex-col">
        {showContextNav ? (
          <SettingsContextNav section={pathname.startsWith("/settings/system") ? "This server" : undefined} />
        ) : null}
        <div className="min-w-0 flex-1 overflow-hidden">{page}</div>
        <SettingsSaveBarHost />
      </div>
    </SettingsEditsProvider>
  );
};

const Route = createFileRoute("/_authed/settings")({
  component: SettingsLayout,
});

export { Route };
