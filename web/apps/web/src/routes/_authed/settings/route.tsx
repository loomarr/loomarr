import * as settingsApi from "@loomarr/api/endpoints/settings";
import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";
import { useAuth } from "@/auth/use-auth";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { NavTabs } from "@/components/ui/nav-tabs";
import { useDocumentTitle } from "@/lib/use-document-title";
import { SettingsEditsProvider } from "@/settings/settings-edits";
import { SettingsSaveBarHost } from "@/settings/settings-save-bar-host";

// Settings (config-design §5) — Sonarr's shape: grouped pages, an explicit save bar per page.
// It is also the troubleshooting console for the life of the install (§13), which is why the
// connection checklist lives on Connections rather than only in the first-run wizard. The page
// nav is a HORIZONTAL tab bar (not a vertical rail beside the app's sidebar — two parallel
// menus read as cluttered); each page is its own route, so the tabs are real links with
// native active-state + deep-linking.
// Curated pages (config-design §5, restructured in V9 to the v2 mock). System is itself sub-tabbed
// — Tasks · Playout · Database · Backup · About — because those are all "the machine" rather
// than "the product", and giving each its own top-level tab buried the four an operator
// actually configures under five they touch once.
const PAGES = [
  { to: "/settings/general", label: "General" },
  { to: "/settings/connections", label: "Connections" },
  { to: "/settings/ai", label: "AI" },
  { to: "/settings/defaults", label: "Defaults" },
  { to: "/settings/notifications", label: "Notifications" },
  { to: "/settings/system", label: "System" },
  { to: "/settings/security", label: "Security" },
  { to: "/settings/all", label: "All settings" },
] as const;

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

  const visiblePages = isAdmin ? PAGES : PAGES.filter((page) => page.to === "/settings/notifications");

  return (
    // ⚠ The provider wraps the OUTLET, which is what makes the save bar cross-tab: the buffer
    // belongs to the layout, so switching pages re-renders the outlet without unmounting the
    // edits. Held one level lower (inside a page) it died on every tab switch.
    <SettingsEditsProvider>
      <div className="flex h-full flex-col">
        {/* ⚠ This bar's look is now the WHOLE APP's (2026-08-02): its pill treatment was
            extracted into the shared `NavTabs`, and Filler and Queue — which drew an underline
            bar — were migrated onto it. The styling that used to live inline here is the
            component's, so a change lands on all three at once.
            ⚠ `activeId` is derived from the PATHNAME because these tabs are separate routes.
            The router's own `data-status="active"` would do it implicitly, but the shared
            component takes an explicit id so the two tab bars that key off `?tab=` and the one
            that keys off the path can use one API. */}
        <NavTabs
          label="Settings"
          linkComponent={Link}
          className="bg-background px-6 pt-2"
          tabs={visiblePages.map((p) => ({ id: p.to, label: p.label, to: p.to }))}
          activeId={visiblePages.find((p) => pathname.startsWith(p.to))?.to ?? visiblePages[0]?.to}
        />
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
