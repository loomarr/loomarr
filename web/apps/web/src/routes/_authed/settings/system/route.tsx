import { createFileRoute, Link, Outlet } from "@tanstack/react-router";
import { cn } from "@/lib";

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
  { to: "/settings/system/database", label: "Database" },
  { to: "/settings/system/backup", label: "Backup" },
  { to: "/settings/system/about", label: "About" },
] as const;

const SystemLayout = () => (
  <div className="flex h-full flex-col">
    <nav aria-label="System settings" className="flex gap-1 overflow-x-auto border-border border-b px-6 py-2">
      {SUB_PAGES.map((p) => (
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

const Route = createFileRoute("/_authed/settings/system")({
  component: SystemLayout,
});

export { Route };
