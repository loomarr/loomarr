import { Link } from "@tanstack/react-router";
import { ArrowLeft, Search } from "lucide-react";

interface SettingsContextNavProps {
  section?: string;
}

// Settings pages deliberately avoid a permanent sibling menu. This small wayfinding row gives
// every leaf a predictable route back to the task home without making all settings look equally
// important again.
const SettingsContextNav = ({ section }: SettingsContextNavProps) => (
  <div className="flex min-h-11 shrink-0 items-center justify-between gap-3 border-border border-b px-4 sm:px-6">
    <nav aria-label="Settings location" className="min-w-0">
      <ol className="flex min-w-0 items-center gap-2 text-sm">
        <li>
          <Link
            to="/settings"
            className="inline-flex items-center gap-1.5 rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <ArrowLeft className="size-3.5" aria-hidden />
            Settings
          </Link>
        </li>
        {section ? (
          <>
            <li className="text-muted-foreground" aria-hidden>
              /
            </li>
            <li className="truncate text-foreground">{section}</li>
          </>
        ) : null}
      </ol>
    </nav>
    <Link
      to="/settings"
      aria-label="Find another setting"
      className="inline-flex shrink-0 items-center gap-1.5 rounded-sm text-muted-foreground text-sm hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <Search className="size-3.5" aria-hidden />
      <span className="hidden sm:inline">Find another setting</span>
      <span className="sm:hidden">Find</span>
    </Link>
  </div>
);

export { SettingsContextNav };
