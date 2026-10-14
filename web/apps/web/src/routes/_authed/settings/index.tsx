import { createFileRoute, Link } from "@tanstack/react-router";
import { ArrowRight, Search, X } from "lucide-react";
import {
  forwardRef,
  type KeyboardEvent,
  type KeyboardEventHandler,
  type ReactNode,
  useMemo,
  useRef,
  useState,
} from "react";
import { useAuth } from "@/auth/use-auth";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { Input } from "@/components/ui/input";
import {
  canVisitDestination,
  SETTINGS_DESTINATIONS,
  SETTINGS_TASK_GROUPS,
  type SettingsDestination,
} from "@/settings/settings-destinations";
import { findSettings } from "@/settings/settings-finder";
import { useSettingsEntries } from "@/settings/use-settings-entries";

interface DestinationAnchorProps {
  destination: SettingsDestination;
  className: string;
  children: ReactNode;
  onKeyDown?: KeyboardEventHandler<HTMLAnchorElement>;
}

const DestinationAnchor = forwardRef<HTMLAnchorElement, DestinationAnchorProps>(
  ({ destination, className, children, onKeyDown }, ref) =>
    destination.section ? (
      <Link
        ref={ref}
        to="/filler/settings/$section"
        params={{ section: destination.section }}
        className={className}
        onKeyDown={onKeyDown}
      >
        {children}
      </Link>
    ) : (
      <Link ref={ref} to={destination.path} className={className} onKeyDown={onKeyDown}>
        {children}
      </Link>
    ),
);
DestinationAnchor.displayName = "DestinationAnchor";

const DestinationLink = ({ destination }: { destination: SettingsDestination }) => (
  <DestinationAnchor
    destination={destination}
    className="group flex min-w-0 items-start gap-3 rounded-lg border border-border bg-card px-4 py-3 transition-colors hover:border-signal/40 hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
  >
    <span className="min-w-0 flex-1">
      <span className="block font-medium text-sm">{destination.label}</span>
      <span className="mt-0.5 block text-muted-foreground text-xs leading-relaxed">
        {destination.description}
      </span>
    </span>
    <ArrowRight
      className="mt-0.5 size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:text-foreground"
      aria-hidden
    />
  </DestinationAnchor>
);

const SettingsHome = () => {
  const { isAdmin } = useAuth();
  const entries = useSettingsEntries();
  const [query, setQuery] = useState("");
  const resultLinks = useRef<Array<HTMLAnchorElement | null>>([]);
  const results = useMemo(() => findSettings({ query, entries, isAdmin }), [entries, isAdmin, query]);
  const hasQuery = query.trim() !== "";

  const moveResultFocus = (event: KeyboardEvent, index: number) => {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      resultLinks.current[Math.min(index + 1, results.length - 1)]?.focus();
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      resultLinks.current[Math.max(index - 1, 0)]?.focus();
    }
  };

  const browseDestinations = (group: string) =>
    SETTINGS_DESTINATIONS.filter(
      (item) => item.browse && item.group === group && canVisitDestination(item, isAdmin),
    );

  const advanced = browseDestinations("advanced")[0];

  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader title="Settings" description="Find what you want to change, or browse by task." />
      <div className="min-h-0 flex-1 overflow-auto">
        <div className="mx-auto max-w-5xl space-y-8 p-6">
          <section aria-labelledby="settings-finder-heading">
            <h2 id="settings-finder-heading" className="font-medium text-base">
              Find a setting
            </h2>
            <p className="mt-1 text-muted-foreground text-sm">
              Search in everyday words, or paste a setting key or environment variable.
            </p>
            <div className="relative mt-3">
              <Search
                className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
                aria-hidden
              />
              <Input
                type="search"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "ArrowDown" && results.length > 0) {
                    event.preventDefault();
                    resultLinks.current[0]?.focus();
                  }
                }}
                placeholder="Try “Jellyfin”, “backups”, or FILLER_FETCH_EVERY"
                aria-label="Find a setting"
                {...(hasQuery ? { "aria-controls": "settings-finder-results" } : {})}
                className="h-11 pr-10 pl-9"
              />
              {hasQuery && (
                <button
                  type="button"
                  onClick={() => setQuery("")}
                  className="absolute top-1/2 right-2 flex size-8 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  aria-label="Clear settings search"
                >
                  <X className="size-4" aria-hidden />
                </button>
              )}
            </div>

            {hasQuery && (
              <div id="settings-finder-results" className="mt-3" aria-live="polite">
                {results.length === 0 ? (
                  <div className="rounded-lg border border-border border-dashed px-4 py-5">
                    <p className="font-medium text-sm">No settings found</p>
                    <p className="mt-1 text-muted-foreground text-sm">
                      Try a shorter phrase, a service name, or an exact key.
                    </p>
                  </div>
                ) : (
                  <div className="overflow-hidden rounded-lg border border-border bg-card">
                    <p className="border-border border-b px-4 py-2 text-muted-foreground text-xs">
                      {results.length} {results.length === 1 ? "match" : "matches"}
                    </p>
                    <nav aria-label="Settings search results" className="divide-y divide-border">
                      {results.slice(0, 10).map((result, index) => (
                        <DestinationAnchor
                          key={result.destination.id}
                          ref={(node) => {
                            resultLinks.current[index] = node;
                          }}
                          destination={result.destination}
                          onKeyDown={(event) => moveResultFocus(event, index)}
                          className="group flex items-center gap-3 px-4 py-3 hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
                        >
                          <span className="min-w-0 flex-1">
                            <span className="block font-medium text-sm">
                              {result.setting?.label || result.destination.label}
                            </span>
                            <span className="mt-0.5 block truncate text-muted-foreground text-xs">
                              {result.groupLabel} · {result.destination.label}
                              {result.setting ? ` · ${result.setting.key}` : ""}
                            </span>
                          </span>
                          <ArrowRight className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                        </DestinationAnchor>
                      ))}
                    </nav>
                  </div>
                )}
              </div>
            )}
          </section>

          <div className="space-y-7">
            {SETTINGS_TASK_GROUPS.map((group) => {
              const destinations = browseDestinations(group.id);
              if (destinations.length === 0) return null;
              return (
                <section key={group.id} aria-labelledby={`settings-group-${group.id}`}>
                  <div className="mb-3">
                    <h2 id={`settings-group-${group.id}`} className="font-medium text-base">
                      {group.label}
                    </h2>
                    <p className="mt-0.5 text-muted-foreground text-xs">{group.description}</p>
                  </div>
                  <div className="grid gap-2 md:grid-cols-2">
                    {destinations.map((item) => (
                      <DestinationLink key={item.id} destination={item} />
                    ))}
                  </div>
                </section>
              );
            })}
          </div>

          {advanced && (
            <section className="border-border border-t pt-5" aria-label="Advanced settings">
              <DestinationAnchor
                destination={advanced}
                className="inline-flex items-center gap-2 text-muted-foreground text-sm hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                Advanced settings
                <ArrowRight className="size-3.5" aria-hidden />
              </DestinationAnchor>
              <p className="mt-1 text-muted-foreground text-xs">
                For exact keys and deployment troubleshooting.
              </p>
            </section>
          )}
        </div>
      </div>
    </div>
  );
};

const Route = createFileRoute("/_authed/settings/")({
  component: SettingsHome,
});

export { Route };
