import type { ReviewAction, ReviewState } from "./review.type";

const initialReview: ReviewState = {
  selected: { filler: [], requests: [] },
  selecting: { filler: false, requests: false },
};

// The approved mock's rules (#1840): Select and Cancel are per group; ticking the first item raises the
// bulk sheet and ticking the last one away lowers it; a result clears that group's selection only.
const reviewReducer = (state: ReviewState, action: ReviewAction): ReviewState => {
  switch (action.type) {
    case "toggle-selecting":
      return {
        selected: { ...state.selected, [action.group]: [] },
        selecting: { ...state.selecting, [action.group]: !state.selecting[action.group] },
      };
    case "toggle-selected": {
      const others = state.selected[action.group].filter((id) => id !== action.id);
      const next = action.on ? [...others, action.id] : others;
      return {
        ...state,
        selected: { ...state.selected, [action.group]: next },
        sheet: next.length > 0 ? { group: action.group, kind: "bulk" } : undefined,
      };
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
