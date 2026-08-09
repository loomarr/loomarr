// The Tune panel's inputs. It reads and writes settings itself, so the only things it takes are
// the two counts the summary line reports — the caller (Incoming) already has them and a second
// query for numbers on screen would be waste.
interface TunePanelProps {
  // Filed is how many clips were filed without asking; NeedsYou is how many are waiting.
  // ⚠ Both are the CURRENT queue's numbers, not lifetime totals: the summary answers "what does
  // this threshold do right now", which is the question an operator moving the chips is asking.
  filed: number;
  needsYou: number;
}

export type { TunePanelProps };
