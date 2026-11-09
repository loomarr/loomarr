import { Icon, icons, type StatusTone } from "@loomarr/design-system";
import { CardStatus, RowCard } from "@loomarr/ui";
import type { RequestEntry } from "@/queue/use-requests";
import type { RequestRowProps } from "./request-row.type";

// A request's badge tone, as a row's status dot. Every list a request is a row in (Requests, Home's
// Your requests) uses the same map, so the same status draws the same dot wherever it shows (#1817 Q11).
const REQUEST_ROW_TONE: Record<RequestEntry["status"]["tone"], StatusTone> = {
  suggest: "attention",
  lock: "ok",
  onair: "error",
  caution: "progress",
};

// RequestRow — one request as a row card (#2765): a request has no picture of its own, so it is the
// row family, not the picture one. The lead is a small channel icon, the title is what was asked, the
// one metadata line starts with the status, and the one action is the next step.
const RequestRow = ({ action, meta = [], status, title }: RequestRowProps) => (
  <RowCard
    action={action}
    lead={<Icon decorative glyph={icons.channels} size="default" tone="secondary" />}
    meta={[<CardStatus key="status" label={status.line} tone={REQUEST_ROW_TONE[status.tone]} />, ...meta]}
    title={title}
  />
);

export { REQUEST_ROW_TONE, RequestRow };
