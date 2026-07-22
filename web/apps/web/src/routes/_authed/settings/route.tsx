import { settingsApi } from "@loomarr/api";
import { createFileRoute, Link, Outlet } from "@tanstack/react-router";
import { ErrorState } from "@/components/loomarr";
import { cn } from "@/lib";

// Settings (config-design §5) — Sonarr's shape: grouped pages, an explicit save bar per page.
// It is also the troubleshooting console for the life of the install (§13), which is why the
// connection checklist lives on Connections rather than only in the first-run wizard. The page
// nav is a HORIZONTAL tab bar (not a vertical rail beside the app's sidebar — two parallel
// menus read as cluttered); each page is its own route, so the tabs are real links with
// native active-state + deep-linking.
const PAGES = [
  { to: "/settings/connections", label: "Connections" },
  { to: "/settings/ai", label: "AI" },
  { to: "/settings/channels", label: "Channels & playback" },
  { to: "/settings/filler", label: "Filler" },
  { to: "/settings/users", label: "Users & security" },
  { to: "/settings/advanced", label: "Advanced" },
] as const;

const SettingsLayout = () => {
  const settings = settingsApi.useSettingsList({ query: { retry: false } });

  if (settings.error) {
    return (
      <div className="p-6">
        <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
      </div>
    );
  }

  return (
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
    </div>
  );
};

const Route = createFileRoute("/_authed/settings")({
  component: SettingsLayout,
});

export { Route };
