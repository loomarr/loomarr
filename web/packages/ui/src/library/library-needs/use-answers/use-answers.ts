import { useCallback, useRef, useState } from "react";
import type { Phase } from "./use-answers.type";

// The answers given here. An answered title locks in and leaves on its own time: the list that
// refetches after the answer no longer holds it, so it is kept where it was until it has left. A
// failed answer puts the question back and says so, as the title page does.
// `restored` counts how often each title has been brought back: a new count forgets its answer.
const useAnswers = <T extends { id: string }>(
  needs: readonly T[],
  restored?: Readonly<Record<string, number>>,
) => {
  const [phases, setPhases] = useState<Record<string, Phase>>({});
  const [seenRestored, setSeenRestored] = useState(restored);
  if (restored !== seenRestored) {
    setSeenRestored(restored);
    setPhases((all) =>
      Object.fromEntries(Object.entries(all).filter(([id]) => restored?.[id] === seenRestored?.[id])),
    );
  }
  const [failed, setFailed] = useState<Record<string, boolean>>({});
  const last = useRef<readonly T[]>([]);
  const fresh = new Map(needs.map((n) => [n.id, n]));
  const kept: T[] = [];
  for (const n of last.current) {
    const now = fresh.get(n.id);
    if (now) kept.push(now);
    else if (phases[n.id] === "added" || phases[n.id] === "ignored") kept.push(n);
  }
  const seen = new Set(kept.map((n) => n.id));
  const list = [...kept, ...needs.filter((n) => !seen.has(n.id))].filter((n) => phases[n.id] !== "gone");
  last.current = list;
  const set = useCallback((id: string, phase?: Phase) => {
    setPhases((all) => {
      const next = { ...all };
      if (phase) next[id] = phase;
      else delete next[id];
      return next;
    });
  }, []);
  const answer = (id: string, phase: "added" | "ignored", save: Promise<void>) => {
    set(id, phase);
    setFailed((all) => ({ ...all, [id]: false }));
    save.catch(() => {
      set(id);
      setFailed((all) => ({ ...all, [id]: true }));
    });
  };
  return { list, phases, failed, answer, done: (id: string) => set(id, "gone") };
};

export { useAnswers };
