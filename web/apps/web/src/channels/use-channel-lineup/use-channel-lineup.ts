import { arrayMove } from "@dnd-kit/sortable";
import type { LineupEntryDTO } from "@loomarr/api/models/lineupEntryDTO";
import { ignoreFailure, useChannelSave } from "@/channels/use-channel-save";
import type { ChannelLineup } from "./use-channel-lineup.type";

// useChannelLineup — the manual lineup editor's operations (§7 PATCH, phase 3 of editable
// channels). Add/remove/reorder are three different mutations of the SAME idea: the backend
// takes a whole-list replace (`PATCH /v1/channels/{id}`, body `{ lineup: LineupEntryDTO[] }`)
// and reconciles server-side, so every op here just computes the next full ordered list and
// saves it — never a per-item endpoint.
//
// SEAMLESS, same as the rest of this feature: no save/rebuild step. Each op saves through the
// channel's one writer (useChannelSave), whose pending lineup this renders at once, so an add,
// remove or drag reads back immediately and the next op builds on it rather than on the stale
// read. Two quick removes, or a remove right after a policy edit, go out one PATCH at a time at
// the right revision (#1899), so the rows stay editable while a save is in flight. A failed save drops the pending lineup, so the list falls back to
// what the server actually has; the writer's toast says why.
const useChannelLineup = (channelId: string, current: LineupEntryDTO[]): ChannelLineup => {
  const { pending, save } = useChannelSave(channelId);
  const entries = pending?.lineup ?? current;
  const commit = (next: LineupEntryDTO[]) => void save({ lineup: next }).catch(ignoreFailure);

  return {
    entries,
    add: (entry) => {
      if (entries.some((e) => e.key === entry.key)) return false;
      commit([...entries, entry]);
      return true;
    },
    remove: (key) => commit(entries.filter((e) => e.key !== key)),
    reorder: (fromIndex, toIndex) => commit(arrayMove(entries, fromIndex, toIndex)),
    updateEntry: (key, patch) => commit(entries.map((e) => (e.key === key ? { ...e, ...patch } : e))),
  };
};

export { useChannelLineup };
