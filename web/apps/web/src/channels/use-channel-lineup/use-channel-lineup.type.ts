import type { LineupEntryDTO } from "@loomarr/api/models/lineupEntryDTO";

interface ChannelLineup {
  // The live lineup as the hook believes it to be — `current` until a commit lands,
  // then whatever the last mutation sent (optimistic; the channel query refetch is
  // what brings it back in line with the server, same as the rest of the page).
  entries: LineupEntryDTO[];
  isPending: boolean;
  // Appends `entry` and commits the whole list. No-ops (returns false) if `entry.key`
  // is already present — the backend 422s on a duplicate key, so this is prevented
  // before the request goes out, not just reported after.
  add: (entry: LineupEntryDTO) => boolean;
  // Drops the entry with `key` and commits the remaining list, in order.
  remove: (key: string) => void;
  // Moves the entry at `fromIndex` to `toIndex` (arrayMove semantics) and commits the
  // reordered list — the shape a dnd-kit `onDragEnd` hands you directly.
  reorder: (fromIndex: number, toIndex: number) => void;
  // Merges `patch` into the entry with `key` and commits — for the per-series season
  // window (§9 "seasons 1–10"). The backend preserves an omitted window by key (§7), so a
  // 0/undefined here means "no constraint", never "season 0".
  updateEntry: (key: string, patch: Partial<LineupEntryDTO>) => void;
}

export type { ChannelLineup };
