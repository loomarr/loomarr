import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";
import { NavTabs } from "@/components/ui/nav-tabs";

// Settings → System (config-design §5, V9) — "the machine", as distinct from "the product".
//
// Sub-tabbed rather than five more top-level tabs: Tasks, Playout, Database, Backup and About
// are all things an operator touches once at setup or when something is wrong, and putting them
// alongside Connections and AI buried the four pages that get configured under five that don't.
//
// Backup and About arrive with V12; Database arrived with V11. Playout is still a later
// phase and its tab is NOT stubbed here: a tab leading to an empty page is worse than an
// absent one — it advertises a surface that does not exist, which is the same reasoning
// §12 used to defer the Dashboard nav entry until V16 built it.
const SUB_PAGES = [
  { to: "/settings/system/tasks", label: "Tasks" },
  { to: "/settings/system/playback", label: "Playback" },
  { to: "/settings/system/database", label: "Database" },
  { to: "/settings/system/backup", label: "Backup" },
  { to: "/settings/system/storage", label: "Storage" },
  { to: "/settings/system/about", label: "About" },
] as const;

const SystemLayout = () => {
  const { pathname } = useLocation();
  const activeId = SUB_PAGES.find((page) => pathname.startsWith(page.to))?.to ?? SUB_PAGES[0].to;

  return (
    <div className="flex h-full flex-col">
      <NavTabs
        label="System settings"
        linkComponent={Link}
        className="bg-background px-6 pt-2"
        tabs={SUB_PAGES.map((page) => ({ id: page.to, label: page.label, to: page.to }))}
        activeId={activeId}
      />
      <div className="min-w-0 flex-1 overflow-hidden">
        <Outlet />
      </div>
    </div>
  );
};

const Route = createFileRoute("/_authed/settings/system")({
  component: SystemLayout,
});

export { Route };
