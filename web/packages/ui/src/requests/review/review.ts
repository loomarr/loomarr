import type { ReviewAction, ReviewGroup, ReviewSheet, ReviewState } from "./review.type";

const initialReview: ReviewState = {
  selected: { filler: [], requests: [] },
  selecting: { filler: false, requests: false },
};

// The one bulk sheet always names a group that still has ticks: `preferred` while it does, otherwise the
// other group. Both groups can be in Select at once, so clearing one must not drop the other's sheet.
const bulkSheetFor = (selected: ReviewState["selected"], preferred: ReviewGroup): ReviewSheet | undefined => {
  const other: ReviewGroup = preferred === "filler" ? "requests" : "filler";
  const group = [preferred, other].find((candidate) => selected[candidate].length > 0);
  return group ? { group, kind: "bulk" } : undefined;
};

// The approved mock's rules (#1840): Select and Cancel are per group; ticking an item raises the bulk
// sheet for its group and ticking the last one away lowers it, or hands it to the other group if that
// still has ticks; a result clears that group's selection only.
const reviewReducer = (state: ReviewState, action: ReviewAction): ReviewState => {
  switch (action.type) {
    case "toggle-selecting": {
      const selected = { ...state.selected, [action.group]: [] };
      return {
        selected,
        selecting: { ...state.selecting, [action.group]: !state.selecting[action.group] },
        sheet: state.sheet?.kind === "bulk" ? bulkSheetFor(selected, state.sheet.group) : undefined,
      };
    }
    case "toggle-selected": {
      const others = state.selected[action.group].filter((id) => id !== action.id);
      const selected = { ...state.selected, [action.group]: action.on ? [...others, action.id] : others };
      return { ...state, selected, sheet: bulkSheetFor(selected, action.group) };
    }
    case "open-deny":
      return { ...state, sheet: { group: action.group, id: action.id, kind: "deny" } };
    case "open-edit":
      return { ...state, sheet: { id: action.id, kind: "edit" } };
    case "show-result":
      return {
        selected: { ...state.selected, [action.group]: [] },
        selecting: { ...state.selecting, [action.group]: false },
        sheet: { group: action.group, kind: "result", rows: action.rows },
      };
    case "close-sheet":
      return { ...state, sheet: undefined };
  }
};

export { initialReview, reviewReducer };
