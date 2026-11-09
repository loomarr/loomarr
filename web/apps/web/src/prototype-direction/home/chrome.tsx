// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// A static stand-in for the app shell around Home: the left rail on desktop, the bottom bar on
// phone. Drawn here (not the real AppShell) because that needs a session and the router's location.
import { Link } from "@tanstack/react-router";
import {
  CalendarClock,
  Ellipsis,
  House,
  Inbox,
  Library,
  type LucideIcon,
  Settings,
  Users,
} from "lucide-react";
import { type ReactNode, useEffect, useRef } from "react";
import { BrandLockup } from "@/components/loomarr/shell/brand-lockup";

interface NavEntry {
  label: string;
  icon: LucideIcon;
}

const RAIL: NavEntry[] = [
  { label: "Home", icon: House },
  { label: "Guide", icon: CalendarClock },
  { label: "Library", icon: Library },
  { label: "Requests", icon: Inbox },
  { label: "People", icon: Users },
  { label: "Settings", icon: Settings },
];

const BAR: NavEntry[] = [
  { label: "Home", icon: House },
  { label: "Guide", icon: CalendarClock },
  { label: "Library", icon: Library },
  { label: "Requests", icon: Inbox },
  { label: "More", icon: Ellipsis },
];

// `?scroll=700` scrolls the content pane, so a screenshot of the fixed-height frame can reach the
// sections below the fold.
const useScrollParam = (ref: React.RefObject<HTMLDivElement | null>) => {
  useEffect(() => {
    const y = Number(new URLSearchParams(window.location.search).get("scroll"));
    if (y > 0) ref.current?.scrollTo({ top: y });
  }, [ref]);
};

// `current` names the lit rail entry (Home unless a route says otherwise, e.g. /prototype/settings);
// on the phone bar, an entry it does not carry lights More.
const AppChrome = ({
  device,
  current = "Home",
  children,
}: {
  device: string;
  current?: string;
  children: ReactNode;
}) => {
  const pane = useRef<HTMLDivElement>(null);
  useScrollParam(pane);
  const barCurrent = BAR.some((e) => e.label === current) ? current : "More";

  if (device === "phone") {
    return (
      <div className="flex h-full flex-col">
        <div ref={pane} className="min-h-0 flex-1 overflow-auto">
          {children}
        </div>
        <nav aria-label="Main" className="border-border bg-background flex h-14 shrink-0 border-t pb-1">
          {BAR.map(({ label, icon: Icon }) => (
            <Link
              key={label}
              to="/dashboard"
              aria-current={label === barCurrent ? "page" : undefined}
              className={`flex flex-1 flex-col items-center justify-center gap-0.5 text-xs ${label === barCurrent ? "text-foreground" : "text-static-400"}`}
            >
              <Icon className="size-5" aria-hidden />
              {label}
            </Link>
          ))}
        </nav>
      </div>
    );
  }

  return (
    <div className="flex h-full">
      <nav aria-label="Main" className="border-border bg-background flex w-56 shrink-0 flex-col border-r p-3">
        <div className="px-2 py-3">
          <BrandLockup variant="compact" />
        </div>
        <ul className="m-0 mt-3 flex list-none flex-col gap-0.5 p-0">
          {RAIL.map(({ label, icon: Icon }) => (
            <li key={label}>
              <Link
                to="/dashboard"
                aria-current={label === current ? "page" : undefined}
                className={`flex h-9 items-center gap-3 rounded-md px-2.5 text-sm ${label === current ? "bg-static-800 text-foreground font-medium" : "text-static-400"}`}
              >
                <Icon className="size-4" aria-hidden />
                {label}
              </Link>
            </li>
          ))}
        </ul>
        <div className="text-static-400 mt-auto flex items-center gap-2 px-2.5 py-2 text-sm">
          <span className="bg-static-800 text-foreground flex size-7 items-center justify-center rounded-full text-xs">
            M
          </span>
          Maya
        </div>
      </nav>
      <div ref={pane} className="min-w-0 flex-1 overflow-auto">
        {children}
      </div>
    </div>
  );
};

export { AppChrome };
