import { Link } from "@tanstack/react-router";
import {
  BadgeInfo,
  CalendarClock,
  Clapperboard,
  House,
  LayoutGrid,
  ListChecks,
  LogOut,
  Search,
  Settings,
  Users,
} from "lucide-react";
import { lazy, Suspense } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { commandShortcutAria, commandShortcutLabel } from "@/lib/platform";
import { usePhoneWidth } from "@/lib/use-phone-width";
import { BrandLockup } from "../brand-lockup";
import type { AppShellProps, NavItem } from "./app-shell.type";

// Lazy, not a static import: PhoneBottomBar is @loomarr/design-system's only consumer of
// TabBar/BottomSheet, which only ever render below `md`. A static import here made AppShell —
// eager for every route — the thing that kept those components (and the primitives chunk they
// pull in) live, tripping scripts/check-fe-bundle.mjs's 1 MiB initial-JS budget even on desktop,
// which never mounts the bar at all. Splitting also needed the package itself marked
// `"sideEffects": false` (web/packages/design-system/package.json): without it, the bundler kept
// index.ts's `export { TabBar } from "./src/tab-bar"` alive for every importer of the package root
// — including main.tsx's eager `LoomarrProvider` — regardless of which specific export a given
// call site used, so the lazy boundary alone changed nothing until that flag let it tree-shake per
// binding.
const PhoneBottomBar = lazy(() =>
  import("../phone-bottom-bar").then((module) => ({ default: module.PhoneBottomBar })),
);

// The ios `TabBar`'s own measured height (web always renders that idiom) — holding this row open
// while the chunk loads keeps the Guide's docked strip (#1795) from jumping up and back down.
const PHONE_BOTTOM_BAR_HEIGHT = 49;

// AppShell — the broadcast-console frame (frontend-design §3). Nav rail + ⌘K entry
// + user menu; content renders in `children`. Admin-only sections are gated by
// `isAdmin` (wired to /v1/auth/me in phase 13.3). The `onair` brand dot nods to
// the product soul; nostalgia stays in the margins (§1).
// TWO AUTHORED NAVS, not one list filtered by role (§12).
//
// This used to be `NAV.filter(i => !i.admin || isAdmin)`, which can only ever present a
// member with the admin's product minus some entries. A member is not a diminished admin —
// they arrive to watch and to ask for things — and the same two routes need DIFFERENT NAMES
// for them: `/suggest` was "Request a channel", `/settings/notifications` is "Notifications". A
// filter cannot rename, so the shape of the old code made the right IA inexpressible.
const ADMIN_NAV: NavItem[] = [
  // Home leads the admin rail, named and drawn as in the #1659 web mock (its screen id is still
  // `dashboard`, and so is the route, so bookmarks keep working).
  { to: "/dashboard", label: "Home", icon: House },
  // Guide IS the channels surface (headed "Channels"): "what do I have" and "what is on" are
  // one grid. The fold completed when the grid grew the origination affordance the mock always
  // specified — `✦ Add a channel` in its header — so `/channels` and `/suggest` are now
  // redirects here rather than doors of their own. `Suggest` needs no admin entry because that
  // exact describe→approve path is inline on this page; the MEMBER nav keeps it, since members
  // have no Guide-header affordance. Seven entries, matching the v2 mock's `navDefs`.
  { to: "/guide", label: "Guide", icon: CalendarClock },
  { to: "/requests", label: "Requests", icon: LayoutGrid },
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
//
// Home leads it too (#1659): the member Home carries no machine state, which is what §11 kept
// from members.
const MEMBER_NAV: NavItem[] = [
  { to: "/dashboard", label: "Home", icon: House },
  { to: "/guide", label: "Guide", icon: CalendarClock },
  { to: "/requests", label: "Requests", icon: LayoutGrid },
  { to: "/settings/notifications", label: "Notifications", icon: Settings },
  { to: "/help", label: "Help", icon: ListChecks },
];

const MAIN_ID = "main-content";

const AppShell = ({
  children,
  isAdmin = true,
  userName = "Operator",
  serverVersion,
  badges,
  onOpenCommand,
  onLogout,
  watchChannelId,
}: AppShellProps) => {
  // Below `md`, the rail is gone in favour of PhoneBottomBar (#1785, Alt A): a flex column with
  // the bar as a normal last row, not `position: fixed`, so it reserves real layout space and the
  // Guide's own docked strip (#1795) naturally ends up above it rather than needing a manual inset.
  const phoneWidth = usePhoneWidth();
  return (
    // `h-screen` + `overflow-hidden`, not `min-h-screen`. With only a MINIMUM the shell grows to
    // fit its content, so every `flex-1 min-h-0 overflow-auto` region inside it inherits an
    // unbounded height and never becomes a scroll viewport — the page scrolls instead. That is
    // invisible on short pages and breaks anything that needs a real viewport: the Guide's
    // virtualizer measured an 11,000px "viewport" and dutifully mounted all 200 rows.
    <div className="flex h-screen flex-col overflow-hidden bg-background text-foreground md:grid md:grid-cols-[auto_1fr]">
      {/* WCAG 2.4.1: the rail puts nine-plus stops before any page content, so the first Tab stop
          jumps past it. Absolute, so it never takes a grid cell; visible only while focused. */}
      <a
        href={`#${MAIN_ID}`}
        className="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-signal focus:px-3 focus:py-2 focus:font-medium focus:text-sm focus:text-static-950"
      >
        Skip to content
      </a>
      {/* `hidden md:flex`, not a width that collapses to icons: below `md` the rail is replaced
          outright by PhoneBottomBar, not shrunk — that collapse is what this PR removes. */}
      <aside className="hidden flex-col gap-1 border-border border-r bg-card px-3 py-4 md:flex md:w-56">
        <nav aria-label="Primary" className="flex w-56 flex-col gap-1">
          <div className="mb-4 px-2">
            <BrandLockup variant="compact" />
          </div>

          <button
            type="button"
            onClick={onOpenCommand}
            aria-label="Open global search"
            aria-keyshortcuts={commandShortcutAria()}
            className="mb-2 flex cursor-pointer items-center gap-2 rounded-md border border-input px-3 py-2 text-muted-foreground text-sm transition-colors hover:bg-accent"
          >
            <Search className="size-4" aria-hidden />
            <span>Search…</span>
            <kbd className="ml-auto font-mono text-static-400 text-xs">{commandShortcutLabel()}</kbd>
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
              <span>{label}</span>
              {/* The v2 mock hangs a `suggest` count off the entry whose surface holds work waiting
                  on the viewer. Absent at zero: a permanent "0" would train the eye to ignore it. */}
              {(badges?.[to] ?? 0) > 0 && (
                <span
                  data-testid={`nav-badge-${to}`}
                  className="ml-auto rounded-full bg-suggest-tint-15 px-[7px] py-px font-mono text-2xs text-suggest-300"
                >
                  {badges?.[to]}
                </span>
              )}
            </Link>
          ))}
        </nav>

        {serverVersion && (
          <Tooltip>
            <TooltipTrigger
              render={
                isAdmin ? (
                  <Link
                    to="/settings/system/about"
                    aria-label={`Loomarr ${serverVersion} — About`}
                    className="mt-auto flex min-w-0 items-center gap-2 rounded-md px-3 py-1.5 font-mono text-static-400 text-xs transition-colors hover:bg-accent hover:text-foreground"
                  />
                ) : (
                  <span className="mt-auto flex min-w-0 items-center gap-2 px-3 py-1.5 font-mono text-static-400 text-xs" />
                )
              }
            >
              <BadgeInfo className="size-4 shrink-0" aria-hidden />
              {isAdmin ? (
                <span className="truncate">{serverVersion}</span>
              ) : (
                <span className="truncate">Loomarr {serverVersion}</span>
              )}
            </TooltipTrigger>
            <TooltipContent side="right">Loomarr {serverVersion}</TooltipContent>
          </Tooltip>
        )}

        <div
          className={`${serverVersion ? "mt-1" : "mt-auto"} flex flex-row gap-2 border-border border-t px-2 pt-3 text-sm`}
        >
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
      </aside>

      {/* `flex flex-col`, not a plain block. A block child is not a flex item, so a page using
          the `min-h-0 flex-1` idiom to fill the viewport gets no constraint from here and grows
          to its content instead — which silently turns any inner `overflow-auto` region into a
          non-scrolling div. `min-h-0` lets this shrink below its content so the OVERFLOW lands
          on the region that asked for it. */}
      {/* `tabIndex={-1}` so the skip link moves focus here, not just the scroll position. */}
      <main
        id={MAIN_ID}
        tabIndex={-1}
        className="flex min-h-0 min-w-0 flex-1 flex-col overflow-auto focus:outline-none"
      >
        {children}
      </main>
      {/* Below `md`, PhoneBottomBar takes this row (#1785): a normal flex sibling of `main`, not
          `position: fixed`, so it reserves real space rather than floating over content. Lazy
          (above): the Suspense fallback holds that row at the bar's own height so layout doesn't
          jump once the design-system chunk finishes loading. */}
      {phoneWidth && (
        <Suspense
          fallback={
            <div data-testid="phone-bottom-bar-placeholder" style={{ height: PHONE_BOTTOM_BAR_HEIGHT }} />
          }
        >
          <PhoneBottomBar badges={badges} isAdmin={isAdmin} watchChannelId={watchChannelId} />
        </Suspense>
      )}
    </div>
  );
};

export { AppShell };
