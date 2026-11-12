import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as jobsApi from "@loomarr/api/endpoints/jobs";
import * as notificationsApi from "@loomarr/api/endpoints/notifications";
import * as systemApi from "@loomarr/api/endpoints/system";
import { Icon, icons, Stack } from "@loomarr/design-system";
import { SearchField, SettingsOverview, type SettingsOverviewCard } from "@loomarr/ui";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  type CSSProperties,
  forwardRef,
  type KeyboardEvent,
  type KeyboardEventHandler,
  type MouseEventHandler,
  type ReactNode,
  useMemo,
  useRef,
  useState,
} from "react";
import { viewerIsAdmin } from "@/auth/me-query";
import { useAuth } from "@/auth/use-auth";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { useRouteContent } from "@/lib/use-route-content";
import {
  cardDestinations,
  destinationById,
  SETTINGS_CARDS,
  type SettingsDestination,
} from "@/settings/settings-destinations";
import { findSettings } from "@/settings/settings-finder";
import { useSettingsStatus } from "@/settings/settings-status";
import { useSettingsEntries } from "@/settings/use-settings-entries";

// The overview renders its own children into this link, with its own style and hover handlers.
interface DestinationAnchorProps {
  destination: SettingsDestination;
  className?: string;
  children?: ReactNode;
  style?: CSSProperties;
  onKeyDown?: KeyboardEventHandler<HTMLAnchorElement>;
  onMouseEnter?: MouseEventHandler<HTMLAnchorElement>;
  onMouseLeave?: MouseEventHandler<HTMLAnchorElement>;
}

const DestinationAnchor = forwardRef<HTMLAnchorElement, DestinationAnchorProps>(
  ({ destination, children, ...anchor }, ref) =>
    destination.section ? (
      <Link ref={ref} to="/filler/settings/$section" params={{ section: destination.section }} {...anchor}>
        {children}
      </Link>
    ) : (
      <Link ref={ref} to={destination.path} {...anchor}>
        {children}
      </Link>
    ),
);
DestinationAnchor.displayName = "DestinationAnchor";

const SettingsHome = () => {
  const { isAdmin } = useAuth();
  // Settings opens on its list of destinations, which needs no data: it is content at once.
  useRouteContent(true);
  const entries = useSettingsEntries();
  const statusOf = useSettingsStatus(isAdmin);
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

  // A member sees only the cards they can visit: a card with no page for them is left out.
  const cards: SettingsOverviewCard[] = SETTINGS_CARDS.flatMap((card) => {
    const pages = cardDestinations(card.id, isAdmin);
    if (pages.length === 0) return [];
    const { status, problem } = statusOf(card.id);
    const fix = problem ? destinationById(problem.destinationId) : undefined;
    return [
      {
        id: card.id,
        title: card.title,
        description: card.description,
        status,
        problem:
          problem && fix
            ? { text: problem.text, pageLabel: fix.label, link: <DestinationAnchor destination={fix} /> }
            : undefined,
        pages: pages.map((page) => {
          const host = page.hostId ? destinationById(page.hostId) : undefined;
          return {
            id: page.id,
            label: page.label,
            note: host ? `on ${host.label}` : undefined,
            link: <DestinationAnchor destination={page} />,
          };
        }),
      },
    ];
  });
  const advanced = cardDestinations("advanced", isAdmin)[0];

  return (
    <Stack className="h-full min-h-0">
      <PageHeader title="Settings" description="Search reaches any setting directly." />
      <div className="min-h-0 flex-1 overflow-auto">
        <SettingsOverview
          advanced={
            advanced
              ? {
                  label: advanced.label,
                  description: advanced.description,
                  link: <DestinationAnchor destination={advanced} />,
                }
              : undefined
          }
          cards={cards}
          search={
            <section aria-label="Find a setting">
              <SearchField
                clearLabel="Clear settings search"
                controls={hasQuery ? "settings-finder-results" : undefined}
                label="Find a setting"
                labelHidden
                onArrowDown={() => resultLinks.current[0]?.focus()}
                onChangeText={setQuery}
                placeholder="Try “Jellyfin”, “backups”, or FILLER_FETCH_EVERY"
                value={query}
              />

              {hasQuery && (
                <div id="settings-finder-results" className="mt-3" aria-live="polite">
                  {results.length === 0 ? (
                    <div className="border-border rounded-lg border border-dashed px-4 py-5">
                      <p className="text-sm font-medium">No settings found</p>
                      <p className="text-muted-foreground mt-1 text-sm">
                        Try a shorter phrase, a service name, or an exact key.
                      </p>
                    </div>
                  ) : (
                    <div className="border-border bg-card overflow-hidden rounded-lg border">
                      <p className="border-border text-muted-foreground border-b px-4 py-2 text-xs">
                        {results.length} {results.length === 1 ? "match" : "matches"}
                      </p>
                      <nav aria-label="Settings search results" className="divide-border divide-y">
                        {results.slice(0, 10).map((result, index) => (
                          <DestinationAnchor
                            key={result.destination.id}
                            ref={(node) => {
                              resultLinks.current[index] = node;
                            }}
                            destination={result.destination}
                            onKeyDown={(event) => moveResultFocus(event, index)}
                            className="group hover:bg-muted/40 focus-visible:ring-ring flex items-center gap-3 px-4 py-3 focus-visible:ring-2 focus-visible:outline-none focus-visible:ring-inset"
                          >
                            <span className="min-w-0 flex-1">
                              <span className="block text-sm font-medium">
                                {result.setting?.label || result.destination.label}
                              </span>
                              <span className="text-muted-foreground mt-0.5 block truncate text-xs">
                                {result.groupLabel} · {result.destination.label}
                                {result.setting ? ` · ${result.setting.key}` : ""}
                              </span>
                            </span>
                            <Icon decorative glyph={icons.goForward} size="compact" tone="muted" />
                          </DestinationAnchor>
                        ))}
                      </nav>
                    </div>
                  )}
                </div>
              )}
            </section>
          }
        />
      </div>
    </Stack>
  );
};

const Route = createFileRoute("/_authed/settings/")({
  component: SettingsHome,
  // The reads behind each card's status word (see `useSettingsStatus`). A card whose check fails
  // settles without a problem, so these are warmed rather than awaited into an error view: a
  // prefetch never rejects. The admin-only reads are skipped for a member, as the page skips them.
  loader: async ({ context: { queryClient } }) => {
    const isAdmin = await viewerIsAdmin(queryClient);
    await Promise.all([
      queryClient.prefetchQuery(
        notificationsApi.getNotificationProvidersListQueryOptions({ query: { retry: false } }),
      ),
      isAdmin ? queryClient.prefetchQuery(diagnosticsApi.getGetCurrentHealthQueryOptions()) : undefined,
      isAdmin ? queryClient.prefetchQuery(jobsApi.getJobsListQueryOptions()) : undefined,
      isAdmin ? queryClient.prefetchQuery(systemApi.getSystemBackupsListQueryOptions()) : undefined,
      isAdmin ? queryClient.prefetchQuery(fillerApi.getFillerReadinessQueryOptions()) : undefined,
    ]);
  },
});

export { Route };
