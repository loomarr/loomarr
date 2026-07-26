import { Link } from "@tanstack/react-router";
import {
  CalendarClock,
  Clapperboard,
  LayoutGrid,
  ListChecks,
  LogOut,
  Search,
  Settings,
  Sparkles,
  Tv,
  Users,
} from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui";
import { BrandLockup } from "../brand-lockup";
import type { AppShellProps, NavItem } from "./app-shell.type";

// AppShell — the broadcast-console frame (frontend-design §3). Nav rail + ⌘K entry
// + user menu; content renders in `children`. Admin-only sections are gated by
// `isAdmin` (wired to /v1/auth/me in phase 13.3). The `onair` brand dot nods to
// the product soul; nostalgia stays in the margins (§1).
// TWO AUTHORED NAVS, not one list filtered by role (§12).
//
// This used to be `NAV.filter(i => !i.admin || isAdmin)`, which can only ever present a
// member with the admin's product minus some entries. A member is not a diminished admin —
// they arrive to watch and to ask for things — and the same two routes need DIFFERENT NAMES
// for them: `/suggest` is "Request a channel", `/queue` is "My requests". A filter cannot
// rename, so the shape of the old code made the right IA inexpressible.
const ADMIN_NAV: NavItem[] = [
  { to: "/channels", label: "Channels", icon: Tv },
  // Channels and Guide answer adjacent questions — "what do I have" and "what is on". The
  // v2 IA folds the first into the second; that is NOT done, because origination ("Add a
  // channel") still lives on the list and the grid has no affordance for it yet. Both doors
  // are real; neither is a redirect.
  { to: "/guide", label: "Guide", icon: CalendarClock },
  { to: "/queue", label: "Queue", icon: LayoutGrid },
  { to: "/suggest", label: "Suggest", icon: Sparkles },
  { to: "/filler", label: "Filler", icon: Clapperboard },
  { to: "/people", label: "People", icon: Users },
  { to: "/settings", label: "Settings", icon: Settings },
  { to: "/help", label: "Help", icon: ListChecks },
];

// The member's four. Same routes, named for what a member is doing with them — and the
// admin-only surfaces are absent entirely rather than present-and-greyed, because a rail of
// dead entries advertises a product they cannot use.
const MEMBER_NAV: NavItem[] = [
  { to: "/guide", label: "Guide", icon: CalendarClock },
  { to: "/suggest", label: "Request a channel", icon: Sparkles },
  { to: "/queue", label: "My requests", icon: LayoutGrid },
  { to: "/help", label: "Help", icon: ListChecks },
];

const AppShell = ({
  children,
  isAdmin = true,
  userName = "Operator",
  onOpenCommand,
  onLogout,
}: AppShellProps) => (
  <div className="grid min-h-screen grid-cols-[auto_1fr] bg-background text-foreground">
    <nav aria-label="Primary" className="flex w-56 flex-col gap-1 border-border border-r bg-card px-3 py-4">
      <div className="mb-4 px-2">
        <BrandLockup variant="compact" />
      </div>

      <button
        type="button"
        onClick={onOpenCommand}
        className="mb-2 flex cursor-pointer items-center gap-2 rounded-md border border-input px-3 py-2 text-muted-foreground text-sm transition-colors hover:bg-accent"
      >
        <Search className="size-4" aria-hidden />
        <span>Search…</span>
        <kbd className="ml-auto font-mono text-static-400 text-xs">⌘K</kbd>
      </button>

      {(isAdmin ? ADMIN_NAV : MEMBER_NAV).map(({ to, label, icon: Icon }) => (
        // TanStack Link marks the matched route with data-status="active" — style the
        // active state off that attribute (higher specificity wins over the base), so
        // AppShell stays a pure-className component (no isActive render-prop).
        <Link
          key={to}
          to={to}
          className="flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm text-static-400 transition-colors hover:bg-accent hover:text-foreground data-[status=active]:bg-signal-tint-15 data-[status=active]:text-signal"
        >
          <Icon className="size-4" aria-hidden />
          {label}
        </Link>
      ))}

      <div className="mt-auto flex items-center gap-2 border-border border-t px-2 pt-3 text-sm">
        {/* The footer identity is the way into Your account (§11) — where the mock puts
            it, and where someone looks for "my settings" rather than the app's. Not a
            NAV item: it isn't a section of the app, it's you. */}
        <Link
          to="/account"
          className="flex min-w-0 flex-1 items-center gap-2 rounded-md p-1 transition-colors hover:bg-accent"
          aria-label="Your account"
        >
          <div className="flex size-7 shrink-0 items-center justify-center rounded-full bg-static-800 font-mono text-xs">
            {userName.slice(0, 2).toUpperCase()}
          </div>
          <span className="truncate text-muted-foreground">{userName}</span>
        </Link>
        {onLogout && (
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                onClick={onLogout}
                aria-label="Sign out"
                className="shrink-0 cursor-pointer rounded-md p-1.5 text-static-400 transition-colors hover:bg-accent hover:text-foreground"
              >
                <LogOut className="size-4" aria-hidden />
              </button>
            </TooltipTrigger>
            <TooltipContent>Sign out</TooltipContent>
          </Tooltip>
        )}
      </div>
    </nav>

    <main className="min-w-0 overflow-auto">{children}</main>
  </div>
);

export { AppShell };
