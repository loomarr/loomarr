import type { LineupGroupKind } from "@loomarr/core/channel-lineup";
import { createChannelListState } from "@loomarr/core/channel-list";
import { useCallback, useState } from "react";

// The channel group the viewer last chose, per browser. Home's wall and the channel list head
// themselves with the same four groups, so both read this one key. A storage that throws, or a value
// that is not a group, means the group a channel list opens on by itself.
const CHANNEL_GROUP_KEY = "loomarr.channels.group";
const KINDS: Record<LineupGroupKind, true> = { all: true, decade: true, favourites: true, recent: true };

const isKind = (value: string | null): value is LineupGroupKind =>
  value !== null && Object.hasOwn(KINDS, value);

const read = (): LineupGroupKind => {
  try {
    const stored = localStorage.getItem(CHANNEL_GROUP_KEY);
    if (isKind(stored)) return stored;
  } catch {
    // Not readable: the default holds.
  }
  return createChannelListState().group;
};

const useChannelGroupPreference = (): [LineupGroupKind, (group: LineupGroupKind) => void] => {
  const [group, setGroup] = useState(read);
  const update = useCallback((next: LineupGroupKind) => {
    setGroup(next);
    try {
      localStorage.setItem(CHANNEL_GROUP_KEY, next);
    } catch {
      // Not persisted; the choice still holds for this page.
    }
  }, []);
  return [group, update];
};

export { CHANNEL_GROUP_KEY, useChannelGroupPreference };
