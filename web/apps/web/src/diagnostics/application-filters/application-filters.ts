import type { EventView } from "@loomarr/api/models/eventView";

// The application log's filters, apart from the view that draws them: the Diagnostics route reads
// and defaults its search params from these when the app starts, so nothing here may pull in UI.

type EventRange = "all" | "1h" | "6h" | "24h";
type EventLevel = "all" | "debug" | "info" | "warn" | "error";
type EventSource = "all" | EventView["source"];
type EventOrder = "newest" | "oldest";

type ApplicationFilters = {
  range: EventRange;
  order: EventOrder;
  level: EventLevel;
  source: EventSource;
  subsystem: string;
  text: string;
  requestId?: string;
  playbackSessionId?: string;
  channelId?: string;
  scheduleBlockId?: string;
  jobId?: string;
  processRunId?: string;
};

const DEFAULT_APPLICATION_FILTERS: ApplicationFilters = {
  range: "all",
  order: "newest",
  level: "all",
  source: "all",
  subsystem: "",
  text: "",
};

// Typed by the generated source enum, so a runtime the server starts recording cannot lack a name.
const sourceNames: Record<EventView["source"], string> = {
  android: "Android phone",
  android_tv: "Android TV",
  apple_tv: "Apple TV",
  ios: "iPhone",
  server: "Loomarr",
  web: "Web player",
};
const eventSources = Object.keys(sourceNames) as EventView["source"][];

export type { ApplicationFilters, EventLevel, EventOrder, EventRange, EventSource };
export { DEFAULT_APPLICATION_FILTERS, eventSources, sourceNames };
