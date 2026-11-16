import { ListDiagnosticEventsLevel } from "@loomarr/api/models/listDiagnosticEventsLevel";
import { ListDiagnosticEventsOrder } from "@loomarr/api/models/listDiagnosticEventsOrder";
import type { ListDiagnosticEventsParams } from "@loomarr/api/models/listDiagnosticEventsParams";
import { ListDiagnosticEventsSource } from "@loomarr/api/models/listDiagnosticEventsSource";
import type { ApplicationFilters, EventRange } from "../../application-filters";

const RANGE_MS: Record<Exclude<EventRange, "all">, number> = {
  "1h": 3_600_000,
  "6h": 21_600_000,
  "24h": 86_400_000,
};

const eventParams = (
  filters: ApplicationFilters,
  cursor?: string,
  now = Date.now(),
): ListDiagnosticEventsParams => {
  const to = now;
  // Zero asks the API to apply its one-hour default, so one millisecond after the epoch is the
  // explicit lower bound for every retained Loomarr event.
  const timeWindow = filters.range === "all" ? { from: 1, to } : { from: to - RANGE_MS[filters.range], to };
  return {
    ...timeWindow,
    limit: 50,
    order: ListDiagnosticEventsOrder[filters.order],
    ...(cursor ? { cursor } : {}),
    ...(filters.level === "all" ? {} : { level: ListDiagnosticEventsLevel[filters.level] }),
    ...(filters.source === "all" ? {} : { source: ListDiagnosticEventsSource[filters.source] }),
    ...(filters.subsystem.trim() ? { subsystem: filters.subsystem.trim() } : {}),
    ...(filters.text.trim() ? { text: filters.text.trim() } : {}),
    ...(filters.requestId ? { requestId: filters.requestId } : {}),
    ...(filters.playbackSessionId ? { playbackSessionId: filters.playbackSessionId } : {}),
    ...(filters.channelId ? { channelId: filters.channelId } : {}),
    ...(filters.scheduleBlockId ? { scheduleBlockId: filters.scheduleBlockId } : {}),
    ...(filters.jobId ? { jobId: filters.jobId } : {}),
    ...(filters.processRunId ? { processRunId: filters.processRunId } : {}),
  };
};

export { eventParams };
