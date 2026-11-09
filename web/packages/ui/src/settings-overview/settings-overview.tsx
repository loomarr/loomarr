import { Icon, icons, StatusWord, Surface, Text, useWindowWidth } from "@loomarr/design-system";
import { cloneElement, type ReactNode, useState } from "react";

import type {
  SettingsOverviewAdvanced,
  SettingsOverviewCard,
  SettingsOverviewLink,
  SettingsOverviewProps,
} from "./settings-overview.type";

// Below this the cards stack in one column (the Direction mock's phone frame).
const TWO_COLUMNS_MIN = 768;
// A long page list (Filler's twelve) shows this many and a count, so every card stays short.
const PAGES_SHOWN = 4;

const fade = "color 120ms, background-color 120ms";

// Renders the caller's router link around our own children, keeping its handlers and style. The
// children are a function of hover, for the links whose colour lifts under the pointer.
const Linked = ({
  children,
  link,
  style,
}: {
  children: (hovered: boolean) => ReactNode;
  link: SettingsOverviewLink;
  style?: (hovered: boolean) => object;
}) => {
  const [hovered, setHovered] = useState(false);
  const own = link.props as {
    onMouseEnter?: (event: unknown) => void;
    onMouseLeave?: (event: unknown) => void;
    style?: object;
  };
  return cloneElement(
    link,
    {
      ...own,
      onMouseEnter: (event: unknown) => {
        own.onMouseEnter?.(event);
        setHovered(true);
      },
      onMouseLeave: (event: unknown) => {
        own.onMouseLeave?.(event);
        setHovered(false);
      },
      style: {
        color: "inherit",
        textDecoration: "none",
        transition: fade,
        ...style?.(hovered),
        ...own.style,
      },
    },
    children(hovered),
  );
};

const Dot = () => (
  <Text aria-hidden as="span" textRole="body">
    ·
  </Text>
);

const ProblemLine = ({ problem }: { problem: NonNullable<SettingsOverviewCard["problem"]> }) => (
  <Surface
    backgroundColor="transparent"
    borderColor="borderDecorative"
    borderRadius={0}
    borderWidth={0}
    borderLeftWidth={2}
    paddingLeft={12}
  >
    <Text as="p" style={{ margin: 0 }} textRole="body">
      {`${problem.text} `}
      <Linked link={problem.link} style={() => ({ textDecoration: "underline", textUnderlineOffset: 2 })}>
        {() => `Open ${problem.pageLabel}`}
      </Linked>
    </Text>
  </Surface>
);

const PageList = ({ pages }: { pages: SettingsOverviewCard["pages"] }) => {
  const shown = pages.slice(0, PAGES_SHOWN);
  const more = pages.length - shown.length;
  const first = pages[0];
  return (
    <Surface
      backgroundColor="transparent"
      borderWidth={0}
      columnGap={4}
      flexDirection="row"
      flexWrap="wrap"
      role="list"
      rowGap={2}
    >
      {shown.map((page, index) => (
        <Surface
          alignItems="center"
          backgroundColor="transparent"
          borderWidth={0}
          flexDirection="row"
          gap={4}
          key={page.id}
          role="listitem"
        >
          {index > 0 && <Dot />}
          <Linked link={page.link} style={() => ({ borderRadius: 4, padding: "2px 4px" })}>
            {() => (
              <Text as="span" textRole="body">
                {page.label}
                {page.note && (
                  <Text as="span" textRole="body" tone="secondary">
                    {` · ${page.note}`}
                  </Text>
                )}
              </Text>
            )}
          </Linked>
        </Surface>
      ))}
      {more > 0 && first && (
        <Surface
          alignItems="center"
          backgroundColor="transparent"
          borderWidth={0}
          flexDirection="row"
          gap={4}
          role="listitem"
        >
          <Dot />
          <Linked link={first.link} style={() => ({ borderRadius: 4, padding: "2px 4px" })}>
            {(hovered) => (
              <Text as="span" textRole="body" tone={hovered ? "primary" : "secondary"}>
                {`${more} more`}
              </Text>
            )}
          </Linked>
        </Surface>
      )}
    </Surface>
  );
};

const Card = ({ card }: { card: SettingsOverviewCard }) => {
  const heading = `settings-card-${card.id}`;
  return (
    <Surface
      aria-labelledby={heading}
      borderRadius="cardCompact"
      flexGrow={1}
      gap={12}
      padding={16}
      role="region"
    >
      <Surface
        alignItems="baseline"
        backgroundColor="transparent"
        borderWidth={0}
        flexDirection="row"
        gap={12}
        justifyContent="space-between"
      >
        <Text as="h2" id={heading} strong style={{ margin: 0 }} textRole="reading">
          {card.title}
        </Text>
        {card.status && <StatusWord kind={card.status} />}
      </Surface>
      {card.problem ? (
        <ProblemLine problem={card.problem} />
      ) : (
        <Text as="p" style={{ margin: 0 }} textRole="body" tone="secondary">
          {card.description}
        </Text>
      )}
      <PageList pages={card.pages} />
    </Surface>
  );
};

const AdvancedRow = ({ advanced }: { advanced: SettingsOverviewAdvanced }) => (
  <Linked
    link={advanced.link}
    style={(hovered) => ({
      alignItems: "center",
      backgroundColor: hovered ? "var(--surfaceElevated)" : "transparent",
      border: "1px dashed var(--borderDecorative)",
      borderRadius: 12,
      boxSizing: "border-box",
      color: "var(--contentPrimary)",
      display: "flex",
      gap: 12,
      padding: "12px 16px",
    })}
  >
    {() => (
      <>
        <Surface backgroundColor="transparent" borderWidth={0} flex={1} minWidth={0}>
          <Text as="span" textRole="label">
            {advanced.label}
          </Text>
          <Text as="span" numberOfLines={1} textRole="cardMeta" tone="secondary">
            {advanced.description}
          </Text>
        </Surface>
        <Icon decorative glyph={icons.disclosure} size="compact" tone="disabled" />
      </>
    )}
  </Linked>
);

const summary = (problems: number): string =>
  problems === 0
    ? "Everything is ready."
    : `${problems} ${problems === 1 ? "thing needs" : "things need"} a look.`;

// SettingsOverview — the Settings index as seven cards plus Advanced (#2786, Direction mock variant
// C). Each card says how it is doing in one status word; a card with a problem says its first one
// in a sentence with a link to the page that fixes it, and those cards come first. Every card
// lists its own pages, so nothing needs opening. One line above the cards counts the problems.
//
// The summary waits for every card's status: a count that grows as the checks land would read as
// things breaking while the page loads.
const SettingsOverview = ({ advanced, cards, search }: SettingsOverviewProps) => {
  const twoColumns = useWindowWidth() >= TWO_COLUMNS_MIN;
  const ordered = [...cards.filter((card) => card.problem), ...cards.filter((card) => !card.problem)];
  const settled = cards.every((card) => card.status !== undefined);
  const problems = cards.filter((card) => card.problem).length;

  return (
    <Surface
      backgroundColor="transparent"
      borderWidth={0}
      gap={24}
      marginHorizontal="auto"
      maxWidth={1024}
      padding={twoColumns ? 32 : 16}
      width="100%"
    >
      {search}
      {settled && (
        <Text as="p" style={{ margin: 0 }} textRole="body" tone="secondary">
          {summary(problems)}
        </Text>
      )}
      <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" flexWrap="wrap" gap={16}>
        {ordered.map((card) => (
          <Surface
            backgroundColor="transparent"
            borderWidth={0}
            key={card.id}
            width={twoColumns ? "calc(50% - 8px)" : "100%"}
          >
            <Card card={card} />
          </Surface>
        ))}
      </Surface>
      {advanced && <AdvancedRow advanced={advanced} />}
    </Surface>
  );
};

export { SettingsOverview };
