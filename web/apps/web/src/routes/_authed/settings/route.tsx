import { settingsApi, setupApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, Outlet } from "@tanstack/react-router";
import { ErrorState, SettingsSaveBar } from "@/components/loomarr";
import { cn, useDocumentTitle } from "@/lib";
import { SettingsEditsProvider, useSettingsEdits } from "@/settings";

// Settings (config-design §5) — Sonarr's shape: grouped pages, an explicit save bar per page.
// It is also the troubleshooting console for the life of the install (§13), which is why the
// connection checklist lives on Connections rather than only in the first-run wizard. The page
// nav is a HORIZONTAL tab bar (not a vertical rail beside the app's sidebar — two parallel
// menus read as cluttered); each page is its own route, so the tabs are real links with
// native active-state + deep-linking.
// Six pages (config-design §5, restructured in V9 to the v2 mock). System is itself sub-tabbed
// — Tasks · Playout · Database · Backup · About — because those are all "the machine" rather
// than "the product", and giving each its own top-level tab buried the four an operator
// actually configures under five they touch once.
const PAGES = [
  { to: "/settings/connections", label: "Connections" },
  { to: "/settings/ai", label: "AI" },
  { to: "/settings/defaults", label: "Defaults" },
  { to: "/settings/system", label: "System" },
  { to: "/settings/security", label: "Security" },
  { to: "/settings/all", label: "All settings" },
] as const;

// The save bar, hoisted to the layout alongside the buffer it commits (V10).
//
// ⚠ V9 lifted the EDITS to the layout but left the BAR inside `SettingsPage`. That held only
// while every tab was a SettingsPage — and V10 adds one that is not (All settings is a table).
// Left there, an edit staged on the new tab had nowhere to be saved from: the buffer took it and
// no Save button existed on screen. If the buffer is layout-owned, so is the bar.
const SettingsSaveBarHost = () => {
  const queryClient = useQueryClient();
  const { edits, resetEdits } = useSettingsEdits();

  const patch = settingsApi.useSettingsPatch({
    mutation: {
      onSuccess: async () => {
        // A saved connection key can flip its check — refresh both, like the wizard.
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: settingsApi.getSettingsListQueryKey() }),
          queryClient.invalidateQueries({ queryKey: setupApi.getSetupStatusQueryKey() }),
        ]);
        resetEdits(); // saved values are the new baseline
      },
    },
  });

  return (
    <>
      {patch.error != null && (
        <div className="px-6 pb-2">
          <ErrorState error={patch.error} />
        </div>
      )}
      <SettingsSaveBar
        dirtyCount={Object.keys(edits).length}
        saving={patch.isPending}
        onDiscard={resetEdits}
        onSave={() => patch.mutate({ data: { edits } })}
      />
    </>
  );
};

const SettingsLayout = () => {
  useDocumentTitle("Settings");
  const settings = settingsApi.useSettingsList({ query: { retry: false } });

  if (settings.error) {
    return (
      <div className="p-6">
        <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
      </div>
    );
  }

  return (
    // ⚠ The provider wraps the OUTLET, which is what makes the save bar cross-tab: the buffer
    // belongs to the layout, so switching pages re-renders the outlet without unmounting the
    // edits. Held one level lower (inside a page) it died on every tab switch.
    <SettingsEditsProvider>
      <div className="flex h-full flex-col">
        {/* Horizontal tab bar — the same tab idiom the channel detail page uses. Scrolls
          sideways on narrow screens; the active page glows brand amber (signal). */}
        <nav
          aria-label="Settings"
          className="flex gap-1 overflow-x-auto border-border border-b bg-background px-6 py-2"
        >
          {PAGES.map((p) => (
            <Link
              key={p.to}
              to={p.to}
              className={cn(
                "shrink-0 whitespace-nowrap rounded-md px-3 py-1.5 text-muted-foreground text-sm transition-colors hover:bg-accent hover:text-foreground",
                "data-[status=active]:bg-signal-tint-15 data-[status=active]:font-medium data-[status=active]:text-signal",
              )}
            >
              {p.label}
            </Link>
          ))}
        </nav>
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
