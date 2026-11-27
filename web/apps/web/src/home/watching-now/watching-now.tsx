import { Grid, SectionHeader, useFocusLook } from "@loomarr/design-system";
import { ProgrammeCard, surfChannelData } from "@loomarr/ui";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { AiringArtwork } from "../channel-wall";
import type { WatchingCard, WatchingNowProps } from "./watching-now.type";

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

// The same two letters the account avatar draws.
const initialsOf = (name: string) => name.slice(0, 2).toUpperCase();

const YOU = "You";

// One card per ACTUAL viewing, the caller's own first (the mock outlines it in amber and leads
// with it). A member's `viewers` holds only their own devices (#1662, Q-H2), so everyone else is
// a count in the header, never a card. A caller who is not watching right now gets no card here —
// their last-tuned channel is Home's separate "Recently tuned" return card, not a stand-in session.
const watchingCards = ({ viewing }: Pick<WatchingNowProps, "viewing">): WatchingCard[] =>
  [...(viewing.viewers ?? [])]
    .sort((a, b) => Number(b.you) - Number(a.you))
    .map((v) => ({
      key: `${v.userId}:${v.device}:${v.channelId}`,
      channelId: v.channelId,
      you: v.you,
      viewer: { name: v.you ? YOU : v.name, initials: initialsOf(v.name), device: v.device },
    }));

const watchingMeta = ({ viewing }: Pick<WatchingNowProps, "viewing">): string | undefined => {
  if (viewing.scope === "household") {
    return plural(new Set((viewing.viewers ?? []).map((v) => v.userId)).size, "person", "people");
  }
  const own = viewing.viewers?.length ?? 0;
  const others = Math.max(0, viewing.watching - own);
  if (own > 0) return others > 0 ? `You and ${plural(others, "other", "others")}` : undefined;
  return others > 0 ? `${others} watching` : undefined;
};

// The card's link. Focus is the card's own amber border (the one the hover draws), so the link
// keeps no outline of its own.
const CardLink = ({
  channelId,
  children,
}: {
  channelId: string;
  children: (focused: boolean) => ReactNode;
}) => {
  const focus = useFocusLook();
  return (
    <Link
      to="/watch/$id"
      params={{ id: channelId }}
      onBlur={focus.onBlur}
      onFocus={focus.onFocus}
      style={{ display: "block", outline: "none" }}
    >
      {children(focus.focused)}
    </Link>
  );
};

// WatchingNow — Home's "Watching now" (#1659 web mock): who is watching what, on which device, as
// the overlay ProgrammeCard over the airing's still. Admins see named viewing across the
// household; members see their own and a count (Q-H2, enforced server-side by #1662).
const WatchingNow = ({ viewing, layout, nowMs }: WatchingNowProps) => {
  const cards = watchingCards({ viewing }).flatMap((card) => {
    const channel = layout.channels.find((c) => c.source.channelId === card.channelId);
    if (!channel) return [];
    const { now, ...identity } = surfChannelData(channel, nowMs, layout.timezone);
    const airing = channel.airings.find((a) => a.source.startMs <= nowMs && a.source.stopMs > nowMs)?.source;
    return now && airing ? [{ ...card, channel, programme: { ...identity, ...now }, airing }] : [];
  });
  if (cards.length === 0) return null;

  return (
    <section aria-labelledby="home-watching">
      <SectionHeader id="home-watching" title="Watching now" meta={watchingMeta({ viewing })} />
      {/* Three across as the mock draws it; fewer as the page narrows, down to one on a phone (#1785). */}
      <Grid columns={{ base: 1, sm: 2, lg: 3 }} gap="3">
        {cards.map(({ key, channel, programme, airing, viewer, you }) => (
          <CardLink key={key} channelId={channel.source.channelId}>
            {(focused) => (
              <ProgrammeCard
                layout="overlay"
                highlighted={you || focused}
                hoverHighlight
                viewer={viewer}
                programme={programme}
                artwork={
                  <AiringArtwork
                    airing={airing}
                    channel={{ name: channel.source.name, number: channel.source.number }}
                    sizes="22rem"
                  />
                }
              />
            )}
          </CardLink>
        ))}
      </Grid>
    </section>
  );
};

export { WatchingNow, watchingCards, watchingMeta };
