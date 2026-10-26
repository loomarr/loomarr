import { Link, useLocation } from "@tanstack/react-router";
import {
  BadgeInfo,
  CalendarClock,
  Clapperboard,
  House,
  Inbox,
  ListChecks,
  LogOut,
  PlayCircle,
  Search,
  Settings,
  Users,
} from "lucide-react";
import { lazy, Suspense } from "react";
import { channelNavHighlight } from "@/channels/channel-nav-highlight";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { commandShortcutAria, commandShortcutLabel } from "@/lib/platform";
import { usePhoneWidth } from "@/lib/use-phone-width";
import { cn } from "@/lib/utils";
import { BrandLockup } from "../brand-lockup";
import type { AppShellProps, NavItem, WatchNavItem } from "./app-shell.type";

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
  { to: "/requests", label: "Requests", icon: Inbox },
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
  { to: "/requests", label: "Requests", icon: Inbox },
  { to: "/settings/notifications", label: "Notifications", icon: Settings },
  { to: "/help", label: "Help", icon: ListChecks },
];

// Watch's rail position (#1817 item 3, decision X2, matching the #1839 phone-bottom-bar draft):
// second, right after Home, in BOTH authored lists — inserted here rather than in ADMIN_NAV/
// MEMBER_NAV themselves because its presence depends on `watchChannelId`, not role.
const navItemsFor = (isAdmin: boolean, watchChannelId: string | undefined): (NavItem | WatchNavItem)[] => {
  const base = isAdmin ? ADMIN_NAV : MEMBER_NAV;
  if (watchChannelId === undefined) return base;
  const watch: WatchNavItem = {
    to: "/channels/$id/watch",
    channelId: watchChannelId,
    label: "Watch",
    icon: PlayCircle,
  };
  return [base[0], watch, ...base.slice(1)].filter(
    (item): item is NavItem | WatchNavItem => item !== undefined,
  );
};

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
  // Decision X2 (critique row 5): any channel's Watch tab highlights Watch; any of its management
  // tabs (Info/Programming/Filler/Danger) highlight Guide instead, so one channel never lights two
  // rail items. `channelNavHighlight` is the one place that rule lives — shared with PhoneBottomBar.
  const { pathname } = useLocation();
  const highlight = channelNavHighlight(pathname);
  return (
    // `h-screen` + `overflow-hidden`, not `min-h-screen`. With only a MINIMUM the shell grows to
    // fit its content, so every `flex-1 min-h-0 overflow-auto` region inside it inherits an
    // unbounded height and never becomes a scroll viewport — the page scrolls instead. That is
    // invisible on short pages and breaks anything that needs a real viewport: the Guide's
    // virtualizer measured an 11,000px "viewport" and dutifully mounted all 200 rows.
    <div className="bg-background text-foreground flex h-screen flex-col overflow-hidden md:grid md:grid-cols-[auto_1fr]">
      {/* WCAG 2.4.1: the rail puts nine-plus stops before any page content, so the first Tab stop
          jumps past it. Absolute, so it never takes a grid cell; visible only while focused. */}
      <a
        href={`#${MAIN_ID}`}
        className="focus:bg-signal focus:text-static-950 sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:px-3 focus:py-2 focus:text-sm focus:font-medium"
      >
        Skip to content
      </a>
      {/* `hidden md:flex`, not a width that collapses to icons: below `md` the rail is replaced
          outright by PhoneBottomBar, not shrunk — that collapse is what this PR removes. */}
      <aside className="border-border bg-card hidden flex-col gap-1 border-r px-3 py-4 md:flex md:w-56">
        <nav aria-label="Primary" className="flex w-56 flex-col gap-1">
          <div className="mb-4 px-2">
            <BrandLockup variant="compact" />
          </div>

          <button
            type="button"
            onClick={onOpenCommand}
            aria-label="Open global search"
            aria-keyshortcuts={commandShortcutAria()}
            className="border-input text-muted-foreground hover:bg-accent mb-2 flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors"
          >
            <Search className="size-4" aria-hidden />
            <span>Search…</span>
            <kbd className="text-static-400 ml-auto font-mono text-xs">{commandShortcutLabel()}</kbd>
          </button>

          {navItemsFor(isAdmin, watchChannelId).map((item) => {
            const { label, icon: Icon } = item;
            // Watch and Guide override TanStack's own path-match active state (`data-status`):
            // Watch's href always points at ONE channel, but any channel's watch route must light
            // it, and Guide must stay lit while the URL has already moved on to a management tab.
            // Every other item keeps the plain automatic match, and so does Guide off a channel route:
            // forcing it to `false` there left /guide itself with no current item.
            const forcedActive =
              item.to === "/channels/$id/watch"
                ? highlight === "watch"
                : item.to === "/guide" && highlight !== undefined
                  ? highlight === "guide"
                  : undefined;
            // "You are here" is neutral, as on the phone bar (#1909 Q1): elevated fill, foreground
            // text at 500 and a 2px foreground edge. Amber is focus only.
            const linkClassName = cn(
              "text-static-400 hover:bg-accent hover:text-foreground flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors",
              forcedActive === undefined
                ? "data-[status=active]:bg-static-800 data-[status=active]:text-foreground data-[status=active]:font-medium data-[status=active]:shadow-[inset_2px_0_0_var(--color-static-100)]"
                : forcedActive &&
                    "bg-static-800 text-foreground font-medium shadow-[inset_2px_0_0_var(--color-static-100)]",
            );
            const content = (
              <>
                <Icon className="size-4" aria-hidden />
                <span>{label}</span>
                {/* The v2 mock hangs a `suggest` count off the entry whose surface holds work
                    waiting on the viewer. Absent at zero: a permanent "0" would train the eye to
                    ignore it. Watch has no badge key (its `to` isn't a stable `NavTo` literal). */}
                {item.to !== "/channels/$id/watch" && (badges?.[item.to] ?? 0) > 0 && (
                  <span
                    data-testid={`nav-badge-${item.to}`}
                    className="bg-suggest-tint-15 text-2xs text-suggest-300 ml-auto rounded-full px-[7px] py-px font-mono"
                  >
                    {badges?.[item.to]}
                  </span>
                )}
              </>
            );
            return item.to === "/channels/$id/watch" ? (
              <Link
                key={`watch-${item.channelId}`}
                to="/channels/$id/watch"
                params={{ id: item.channelId }}
                className={linkClassName}
              >
                {content}
              </Link>
            ) : (
              <Link key={item.to} to={item.to} className={linkClassName}>
                {content}
              </Link>
            );
          })}
        </nav>

        {serverVersion && (
          <Tooltip>
            <TooltipTrigger
              render={
                isAdmin ? (
                  <Link
                    to="/settings/system/about"
                    aria-label={`Loomarr ${serverVersion} — About`}
                    className="text-static-400 hover:bg-accent hover:text-foreground mt-auto flex min-w-0 items-center gap-2 rounded-md px-3 py-1.5 font-mono text-xs transition-colors"
                  />
                ) : (
                  <span className="text-static-400 mt-auto flex min-w-0 items-center gap-2 px-3 py-1.5 font-mono text-xs" />
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
          className={`${serverVersion ? "mt-1" : "mt-auto"} border-border flex flex-row gap-2 border-t px-2 pt-3 text-sm`}
        >
          {/* The footer identity is the way into Your account (§11) — where the mock puts
              it, and where someone looks for "my settings" rather than the app's. Not a
              NAV item: it isn't a section of the app, it's you. */}
          <Link
            to="/account"
            className="hover:bg-accent flex min-w-0 flex-1 items-center gap-2 rounded-md p-1 transition-colors"
            aria-label="Your account"
          >
            <div className="bg-static-800 flex size-7 shrink-0 items-center justify-center rounded-full font-mono text-xs">
              {userName.slice(0, 2).toUpperCase()}
            </div>
            <span className="text-muted-foreground truncate">{userName}</span>
          </Link>
          {onLogout && (
            <Tooltip>
              <TooltipTrigger
                render={
                  <button
                    type="button"
                    onClick={onLogout}
                    aria-label="Sign out"
                    className="text-static-400 hover:bg-accent hover:text-foreground shrink-0 cursor-pointer rounded-md p-1.5 transition-colors"
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
          jump once the design-system chunk finishes loading. A `<nav aria-label="Primary">`
          landmark wraps it — matching the rail's own landmark name — since the rail's `<nav>`
          is `hidden` below `md`, not removed from PhoneBottomBar's side: without this, phone
          width has no "Primary" navigation landmark at all. `TabBar`'s own `role="tablist"` stays
          inside it; a landmark wrapping a widget is an ordinary ARIA pattern. */}
      {phoneWidth && (
        <nav aria-label="Primary">
          <Suspense
            fallback={
              <div data-testid="phone-bottom-bar-placeholder" style={{ height: PHONE_BOTTOM_BAR_HEIGHT }} />
            }
          >
            <PhoneBottomBar badges={badges} isAdmin={isAdmin} watchChannelId={watchChannelId} />
          </Suspense>
        </nav>
      )}
    </div>
  );
};

export { AppShell };
