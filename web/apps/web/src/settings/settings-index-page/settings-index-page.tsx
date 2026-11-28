import { Card, Icon, icons, PressableRow, Stack, Text } from "@loomarr/design-system";
import { SearchField, SettingsOverview, type SettingsOverviewCard } from "@loomarr/ui";
import { Link } from "@tanstack/react-router";
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

const SettingsIndexPage = () => {
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
    <Stack height="100%" minHeight={0}>
      <PageHeader title="Settings" description="Search reaches any setting directly." />
      <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minHeight={0} overflow="auto">
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
                <Stack aria-live="polite" id="settings-finder-results" marginTop={12}>
                  {results.length === 0 ? (
                    <Stack
                      borderColor="borderDecorative"
                      borderRadius={12}
                      borderStyle="dashed"
                      borderWidth={1}
                      gap="1"
                      paddingHorizontal={16}
                      paddingVertical={20}
                    >
                      <Text as="p" textRole="cardLabel">
                        No settings found
                      </Text>
                      <Text as="p" textRole="compact" tone="secondary">
                        Try a shorter phrase, a service name, or an exact key.
                      </Text>
                    </Stack>
                  ) : (
                    <Card overflow="hidden">
                      <Stack
                        borderBottomWidth={1}
                        borderColor="borderDecorative"
                        paddingHorizontal={16}
                        paddingVertical={8}
                      >
                        <Text as="p" textRole="hint" tone="secondary">
                          {results.length} {results.length === 1 ? "match" : "matches"}
                        </Text>
                      </Stack>
                      <Stack aria-label="Settings search results" render="nav">
                        {results.slice(0, 10).map((result, index, shown) => (
                          <PressableRow
                            divided={index < shown.length - 1}
                            inset={{ x: 16, y: 12 }}
                            key={result.destination.id}
                            ref={(node) => {
                              // On the web the row is the anchor `render` draws.
                              resultLinks.current[index] = node as unknown as HTMLAnchorElement | null;
                            }}
                            render={
                              <DestinationAnchor
                                destination={result.destination}
                                onKeyDown={(event) => moveResultFocus(event, index)}
                              />
                            }
                          >
                            <Stack
                              flexBasis="0%"
                              flexGrow={1}
                              flexShrink={1}
                              gap="0.5"
                              minWidth={0}
                              render="span"
                            >
                              <Text as="span" textRole="cardLabel">
                                {result.setting?.label || result.destination.label}
                              </Text>
                              <Text as="span" numberOfLines={1} textRole="hint" tone="secondary">
                                {result.groupLabel} · {result.destination.label}
                                {result.setting ? ` · ${result.setting.key}` : ""}
                              </Text>
                            </Stack>
                            <Icon decorative glyph={icons.goForward} size="compact" tone="muted" />
                          </PressableRow>
                        ))}
                      </Stack>
                    </Card>
                  )}
                </Stack>
              )}
            </section>
          }
        />
      </Stack>
    </Stack>
  );
};

export { SettingsIndexPage };
