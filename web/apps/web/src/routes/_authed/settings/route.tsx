import { settingsApi } from "@loomarr/api";
import { createFileRoute, Link, Outlet } from "@tanstack/react-router";
import { ErrorState } from "@/components/loomarr";
import { cn } from "@/lib";

// Settings (config-design §5) — Sonarr's shape: grouped pages down the side, an explicit
// save bar per page. It is also the troubleshooting console for the life of the install
// (§13), which is why the connection checklist lives on Connections rather than only in
// the first-run wizard.
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
    <div className="grid h-full grid-cols-[14rem_1fr]">
      <nav aria-label="Settings" className="flex flex-col gap-1 border-border border-r p-4">
        {PAGES.map((p) => (
          <Link
            key={p.to}
            to={p.to}
            className={cn(
              "rounded-md px-3 py-2 text-sm text-static-400 transition-colors hover:bg-accent hover:text-foreground",
              "data-[status=active]:bg-signal-tint-15 data-[status=active]:text-signal",
            )}
          >
            {p.label}
          </Link>
        ))}
      </nav>
      <div className="min-w-0">
        <Outlet />
      </div>
    </div>
  );
};

const Route = createFileRoute("/_authed/settings")({
  component: SettingsLayout,
});

export { Route };
