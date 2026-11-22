import type { EventView } from "@loomarr/api/models/eventView";
import type { StatusTone } from "@loomarr/design-system";
import { sourceNames } from "../../application-filters";

const formatTime = (millis: number) =>
  new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "medium" }).format(new Date(millis));

const levelTone = (level: EventView["level"]): StatusTone => {
  if (level === "error") return "error";
  if (level === "warn") return "warn";
  if (level === "info") return "ok";
  return "off";
};

const levelLabel = (level: EventView["level"]) => {
  if (level === "warn") return "Warning";
  return `${level.charAt(0).toUpperCase()}${level.slice(1)}`;
};

const sourceLabel = (source: EventView["source"]) => sourceNames[source];

const fallbackMessage = (event: EventView) =>
  event.event
    .split(/[._]/)
    .filter(Boolean)
    .map((part) => `${part.charAt(0).toUpperCase()}${part.slice(1)}`)
    .join(" ");

export { fallbackMessage, formatTime, levelLabel, levelTone, sourceLabel };
