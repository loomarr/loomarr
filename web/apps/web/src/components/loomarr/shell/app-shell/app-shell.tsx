import { Link } from "@tanstack/react-router";
import {
  CalendarClock,
  Clapperboard,
  LayoutDashboard,
  LayoutGrid,
  ListChecks,
  LogOut,
  Search,
  Settings,
  Users,
} from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
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
  // Dashboard leads the admin rail, matching the v2 mock. It was deferred while it did not
  // exist ("a nav entry to a placeholder is worse than no entry", §12) — V16 built the
  // surface, so the entry is now a door rather than a promise. Members do not get it: its
  // content is machine state, which §11 keeps to admins.
  { to: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  // Guide IS the channels surface (headed "Channels"): "what do I have" and "what is on" are
  // one grid. The fold completed when the grid grew the origination affordance the mock always
  // specified — `✦ Add a channel` in its header — so `/channels` and `/suggest` are now
  // redirects here rather than doors of their own. `Suggest` needs no admin entry because that
  // exact describe→approve path is inline on this page; the MEMBER nav keeps it, since members
  // have no Guide-header affordance. Seven entries, matching the v2 mock's `navDefs`.
  { to: "/guide", label: "Guide", icon: CalendarClock },
  { to: "/queue", label: "Queue", icon: LayoutGrid },
  { to: "/filler", label: "Filler", icon: Clapperboard },
  { to: "/people", label: "People", icon: Users },
  { to: "/settings", label: "Settings", icon: Settings },
  { to: "/help", label: "Help", icon: ListChecks },
];

// The member's four. Same routes, named for what a member is doing with them — and the
// admin-only surfaces are absent entirely rather than present-and-greyed, because a rail of
// dead entries advertises a product they cannot use.
// THREE, not the four §12 recorded pre-fold. `Request a channel` was a nav entry only while
// `/suggest` was a separate page; it folded into the Guide header, where members get the same
// affordance (labelled "Request a channel" for them — it is the only origination door in the
// app, so a member who cannot reach it cannot ask for anything). Listing it again here would
// be a second link to /guide: a duplicate React key and two entries highlighting active at
// once, not an IA choice. The verb lives on the surface it acts on.
const MEMBER_NAV: NavItem[] = [
  { to: "/guide", label: "Guide", icon: CalendarClock },
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
  // `h-screen` + `overflow-hidden`, not `min-h-screen`. With only a MINIMUM the shell grows to
  // fit its content, so every `flex-1 min-h-0 overflow-auto` region inside it inherits an
  // unbounded height and never becomes a scroll viewport — the page scrolls instead. That is
  // invisible on short pages and breaks anything that needs a real viewport: the Guide's
  // virtualizer measured an 11,000px "viewport" and dutifully mounted all 200 rows.
  <div className="grid h-screen grid-cols-[auto_1fr] overflow-hidden bg-background text-foreground">
    <nav
      aria-label="Primary"
      className="flex w-14 flex-col gap-1 border-border border-r bg-card px-1 py-4 md:w-56 md:px-3"
    >
      <div className="mb-4 px-2 text-center md:text-left">
        <span className="font-semibold md:hidden" aria-hidden>
          L
        </span>
        <div className="hidden md:block">
          <BrandLockup variant="compact" />
        </div>
      </div>

      <button
        type="button"
        onClick={onOpenCommand}
        aria-label="Open global search"
        className="mb-2 flex cursor-pointer items-center justify-center gap-2 rounded-md border border-input px-2 py-2 text-muted-foreground text-sm transition-colors hover:bg-accent md:justify-start md:px-3"
      >
        <Search className="size-4" aria-hidden />
        <span className="sr-only md:not-sr-only">Search…</span>
        <kbd className="ml-auto hidden font-mono text-static-400 text-xs md:inline">⌘K</kbd>
      </button>

      {(isAdmin ? ADMIN_NAV : MEMBER_NAV).map(({ to, label, icon: Icon }) => (
        // TanStack Link marks the matched route with data-status="active" — style the
        // active state off that attribute (higher specificity wins over the base), so
        // AppShell stays a pure-className component (no isActive render-prop).
        <Link
          key={to}
          to={to}
          className="flex cursor-pointer items-center justify-center gap-3 rounded-md px-2 py-2 text-sm text-static-400 transition-colors hover:bg-accent hover:text-foreground data-[status=active]:bg-signal-tint-15 data-[status=active]:text-signal md:justify-start md:px-3"
        >
          <Icon className="size-4" aria-hidden />
          <span className="sr-only md:not-sr-only">{label}</span>
        </Link>
      ))}

      <div className="mt-auto flex flex-col items-center gap-2 border-border border-t px-1 pt-3 text-sm md:flex-row md:px-2">
        {/* The footer identity is the way into Your account (§11) — where the mock puts
            it, and where someone looks for "my settings" rather than the app's. Not a
            NAV item: it isn't a section of the app, it's you. */}
        <Link
          to="/account"
          className="flex min-w-0 items-center gap-2 rounded-md p-1 transition-colors hover:bg-accent md:flex-1"
          aria-label="Your account"
        >
          <div className="flex size-7 shrink-0 items-center justify-center rounded-full bg-static-800 font-mono text-xs">
            {userName.slice(0, 2).toUpperCase()}
          </div>
          <span className="sr-only truncate text-muted-foreground md:not-sr-only">{userName}</span>
        </Link>
        {onLogout && (
          <Tooltip>
            <TooltipTrigger
              render={
                <button
                  type="button"
                  onClick={onLogout}
                  aria-label="Sign out"
                  className="shrink-0 cursor-pointer rounded-md p-1.5 text-static-400 transition-colors hover:bg-accent hover:text-foreground"
                />
              }
            >
              <LogOut className="size-4" aria-hidden />
            </TooltipTrigger>
            <TooltipContent>Sign out</TooltipContent>
          </Tooltip>
        )}
      </div>
    </nav>

    {/* `flex flex-col`, not a plain block. A block child is not a flex item, so a page using
        the `min-h-0 flex-1` idiom to fill the viewport gets no constraint from here and grows
        to its content instead — which silently turns any inner `overflow-auto` region into a
        non-scrolling div. `min-h-0` lets this shrink below its content so the OVERFLOW lands
        on the region that asked for it. */}
    <main className="flex min-h-0 min-w-0 flex-col overflow-auto">{children}</main>
  </div>
);

export { AppShell };
