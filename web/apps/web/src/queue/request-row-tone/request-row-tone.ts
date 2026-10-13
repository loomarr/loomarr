import type { RequestStatus } from "@loomarr/core/requests";
import type { StatusTone } from "@/components/ui/status-dot";

// A request's badge tone, as a row's status dot. One map for every place a request is a
// `ListRow` (Home's Your requests, Requests › In progress and Done), so the same status draws the
// same dot wherever it appears (#1817 Q11).
const REQUEST_ROW_TONE: Record<RequestStatus["tone"], StatusTone> = {
  suggest: "attention",
  lock: "ok",
  onair: "error",
  caution: "progress",
};

export { REQUEST_ROW_TONE };
