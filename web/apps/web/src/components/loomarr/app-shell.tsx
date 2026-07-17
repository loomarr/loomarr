import { NavLink } from "react-router-dom";
import { Clapperboard, LayoutGrid, ListChecks, Radio, Search, Settings, Sparkles, Tv, Users } from "lucide-react";
import { cn } from "@/lib/utils";

// AppShell — the broadcast-console frame (frontend-design §3). Nav rail +
// ⌘K entry + user menu; content renders in `children`. Admin-only sections are
// gated by `isAdmin` (wired to /v1/auth/me in phase 13.3). The `onair` brand dot
// nods to the product soul; nostalgia stays in the margins (§1).
export interface NavItem {
  to: string;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  admin?: boolean;
}

const NAV: NavItem[] = [
  { to: "/channels", label: "Channels", icon: Tv },
  { to: "/board", label: "Board", icon: LayoutGrid },
  { to: "/suggest", label: "Suggest", icon: Sparkles },
  { to: "/filler", label: "Filler", icon: Clapperboard },
  { to: "/users", label: "Users", icon: Users, admin: true },
  { to: "/settings", label: "Settings", icon: Settings, admin: true },
  { to: "/help", label: "Help", icon: ListChecks },
];

export function AppShell({
  children,
  isAdmin = true,
  userName = "Operator",
  onOpenCommand,
}: {
  children: React.ReactNode;
  isAdmin?: boolean;
  userName?: string;
  onOpenCommand?: () => void;
}) {
  return (
    <div className="grid min-h-screen grid-cols-[auto_1fr] bg-background text-foreground">
      <nav
        aria-label="Primary"
        className="flex w-56 flex-col gap-1 border-r border-border bg-card px-3 py-4"
      >
        <div className="mb-4 flex items-center gap-2 px-2">
          <Radio className="size-5 text-signal" aria-hidden />
          <span className="font-mono text-md font-semibold tracking-tight">Loomarr</span>
        </div>

        <button
          type="button"
          onClick={onOpenCommand}
          className="mb-2 flex items-center gap-2 rounded-md border border-input px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent"
        >
          <Search className="size-4" aria-hidden />
          <span>Search…</span>
          <kbd className="ml-auto font-mono text-xs text-static-400">⌘K</kbd>
        </button>

        {NAV.filter((i) => !i.admin || isAdmin).map(({ to, label, icon: Icon }) => (
          <NavLink
            key={to}
            to={to}
            className={({ isActive }) =>
              cn(
                "flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors",
                isActive
                  ? "bg-signal-tint-15 text-signal"
                  : "text-static-400 hover:bg-accent hover:text-foreground",
              )
            }
          >
            <Icon className="size-4" aria-hidden />
            {label}
          </NavLink>
        ))}

        <div className="mt-auto flex items-center gap-2 border-t border-border px-2 pt-3 text-sm">
          <div className="flex size-7 items-center justify-center rounded-full bg-static-800 font-mono text-xs">
            {userName.slice(0, 2).toUpperCase()}
          </div>
          <span className="truncate text-muted-foreground">{userName}</span>
        </div>
      </nav>

      <main className="min-w-0 overflow-auto">{children}</main>
    </div>
  );
}
