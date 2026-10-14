import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import {
  canVisitDestination,
  destinationForOwner,
  SETTINGS_DESTINATIONS,
  SETTINGS_TASK_GROUPS,
  type SettingsDestination,
} from "../settings-destinations";

interface SettingsFinderResult {
  destination: SettingsDestination;
  setting?: SettingEntry;
  groupLabel: string;
  score: number;
}

interface FindSettingsInput {
  query: string;
  entries: readonly SettingEntry[];
  isAdmin: boolean;
}

const normalize = (value: string): string => value.toLocaleLowerCase().trim().replace(/\s+/g, " ");

const groupLabel = (destination: SettingsDestination): string =>
  SETTINGS_TASK_GROUPS.find((group) => group.id === destination.group)?.label ??
  (destination.group === "advanced" ? "Advanced" : "Settings");

const scoreFields = (
  query: string,
  priority: readonly string[],
  supporting: readonly string[],
): number | undefined => {
  const normalizedPriority = priority.map(normalize).filter(Boolean);
  const normalizedSupporting = supporting.map(normalize).filter(Boolean);
  const all = [...normalizedPriority, ...normalizedSupporting];
  const tokens = query.split(" ").filter(Boolean);
  if (!tokens.every((token) => all.some((field) => field.includes(token)))) return undefined;

  if (normalizedPriority.some((field) => field === query)) return 0;
  if (normalizedPriority.some((field) => field.startsWith(query))) return 1;
  if (normalizedPriority.some((field) => field.includes(query))) return 2;
  if (normalizedSupporting.some((field) => field === query)) return 3;
  if (normalizedSupporting.some((field) => field.startsWith(query))) return 4;
  if (normalizedSupporting.some((field) => field.includes(query))) return 5;
  return 6;
};

const destinationScore = (query: string, destination: SettingsDestination): number | undefined =>
  scoreFields(
    query,
    [destination.label, ...destination.aliases],
    [destination.description, groupLabel(destination)],
  );

const settingScore = (
  query: string,
  destination: SettingsDestination,
  setting: SettingEntry,
): number | undefined => {
  const enumMetadata = [
    ...(setting.enum ?? []),
    ...(setting.enumOptions ?? []).flatMap((option) => [option.label, option.value]),
  ];
  return scoreFields(
    query,
    [setting.key, setting.envVar ?? "", setting.label ?? "", destination.label, ...destination.aliases],
    [
      setting.doc,
      setting.group,
      setting.kind,
      setting.presentation ?? "",
      ...enumMetadata,
      destination.description,
      groupLabel(destination),
    ],
  );
};

// The finder navigates to existing owners; it never edits or searches resolved values. Omitting
// values is both safer for secrets and avoids matching a user's URLs or tokens as navigation copy.
const findSettings = ({ query, entries, isAdmin }: FindSettingsInput): SettingsFinderResult[] => {
  const normalizedQuery = normalize(query);
  if (!normalizedQuery) return [];

  const bestByDestination = new Map<string, SettingsFinderResult>();
  const consider = (result: SettingsFinderResult) => {
    const current = bestByDestination.get(result.destination.id);
    if (
      !current ||
      result.score < current.score ||
      (result.score === current.score && result.setting && !current.setting)
    ) {
      bestByDestination.set(result.destination.id, result);
    }
  };

  for (const destination of SETTINGS_DESTINATIONS) {
    if (!canVisitDestination(destination, isAdmin)) continue;
    const score = destinationScore(normalizedQuery, destination);
    if (score !== undefined) consider({ destination, groupLabel: groupLabel(destination), score });
  }

  for (const setting of entries) {
    const destination = destinationForOwner(setting.owner);
    if (!destination || !canVisitDestination(destination, isAdmin)) continue;
    const score = settingScore(normalizedQuery, destination, setting);
    if (score !== undefined) {
      consider({ destination, setting, groupLabel: groupLabel(destination), score });
    }
  }

  return [...bestByDestination.values()].sort(
    (left, right) =>
      left.score - right.score || left.destination.label.localeCompare(right.destination.label),
  );
};

export type { FindSettingsInput, SettingsFinderResult };
export { findSettings };
