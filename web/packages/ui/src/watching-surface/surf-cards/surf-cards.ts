import type { GuideLayout } from "@loomarr/core/guide";
import { useEffect, useRef, useState } from "react";

import { surfChannelData } from "../../surf-rail/surf-data";
import type { PlayerSurf, PlayerSurfChannel } from "../player-chrome/player-chrome.type";

// Flipping channels (§9.2, §9.4): every flip tunes at once, and the row of channel cards rides along
// in the band's place, from a flip until a beat after the last one. Every client reads the cards the
// same way: the channels a flip steps through, in that order, each with what is on now from the guide.

/** How long the cards stay after the last flip. */
const SURF_LINGER_MS = 1_200;

/** A channel a flip steps through: the player's catalog entry. */
interface SurfCatalogChannel {
  id: string;
  name: string;
  number: number;
}

/** The cards for every channel in the catalog, with what each has on now where the guide knows it. */
const surfCardsFromGuide = (
  catalog: readonly SurfCatalogChannel[],
  layout: GuideLayout | undefined,
  nowMs: number,
): PlayerSurfChannel[] => {
  const guide = new Map(layout?.channels.map((channel) => [channel.source.channelId, channel]));
  return catalog.map((channel) => {
    const row = guide.get(channel.id);
    const now = row ? surfChannelData(row, nowMs, layout?.timezone).now : undefined;
    return {
      id: channel.id,
      name: channel.name,
      number: String(channel.number),
      ...(row?.source.decade === undefined ? {} : { decade: row.source.decade }),
      ...(now ? { nowPlaying: now.title } : {}),
      ...(now?.artworkUri ? { artUri: now.artworkUri } : {}),
    };
  });
};

/**
 * The cards while channels are being flipped: from a flip until SURF_LINGER_MS after the last one,
 * otherwise undefined. `flipKey` changes with every flip (the channel a step tuned, or a count of
 * presses) and is undefined when the last tune was not a flip, which puts the cards away at once.
 */
const useSurfCards = (
  cards: readonly PlayerSurfChannel[],
  tunedId: string | undefined,
  flipKey: number | string | undefined,
): PlayerSurf | undefined => {
  const [up, setUp] = useState(false);
  // The key as the player first showed it: a flip made before this mounted is not one to show.
  const seen = useRef(flipKey);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => {
    if (flipKey === seen.current) return;
    seen.current = flipKey;
    clearTimeout(timer.current);
    if (flipKey === undefined) {
      setUp(false);
      return;
    }
    setUp(true);
    timer.current = setTimeout(() => setUp(false), SURF_LINGER_MS);
  }, [flipKey]);
  useEffect(() => () => clearTimeout(timer.current), []);
  return up && tunedId !== undefined && cards.length > 1 ? { channels: cards, tunedId } : undefined;
};

export type { SurfCatalogChannel };
export { SURF_LINGER_MS, surfCardsFromGuide, useSurfCards };
