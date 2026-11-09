import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as jobsApi from "@loomarr/api/endpoints/jobs";
import * as notificationsApi from "@loomarr/api/endpoints/notifications";
import * as systemApi from "@loomarr/api/endpoints/system";
import type { BackupList } from "@loomarr/api/models/backupList";
import type { FillerStorageStatusDTO } from "@loomarr/api/models/fillerStorageStatusDTO";
import type { HealthCheck } from "@loomarr/api/models/healthCheck";
import type { HealthReport } from "@loomarr/api/models/healthReport";
import type { JobView } from "@loomarr/api/models/jobView";
import type { NotificationProviderDTO } from "@loomarr/api/models/notificationProviderDTO";
import { unwrap } from "@loomarr/api/unwrap";
import type { StatusWordKind } from "@loomarr/design-system";
import { missingNights } from "@/components/loomarr/settings/backup-panel/missing-nights";
import { SETTINGS_DESTINATIONS, type SettingsCardId } from "../settings-destinations";

type SettingsProblemKind = Exclude<StatusWordKind, "ok">;

interface SettingsProblem {
  // The destination that fixes it; its card is where the problem shows.
  destinationId: string;
  card: SettingsCardId;
  kind: SettingsProblemKind;
  // One sentence in plain words, ending in a full stop.
  text: string;
}

interface SettingsStatusInput {
  health?: HealthReport;
  jobs?: readonly JobView[];
  backups?: BackupList;
  fillerStorage?: FillerStorageStatusDTO;
  notifications?: readonly NotificationProviderDTO[];
}

// Worst first: a card's status word is its worst problem's, and that problem is the one it states.
const SEVERITY: Record<SettingsProblemKind, number> = { attention: 0, overdue: 1, expired: 2, notSetUp: 3 };

const destinationCard = (destinationId: string): SettingsCardId =>
  SETTINGS_DESTINATIONS.find((item) => item.id === destinationId)?.card ?? "server";

const problem = (destinationId: string, kind: SettingsProblemKind, text: string): SettingsProblem => ({
  destinationId,
  card: destinationCard(destinationId),
  kind,
  text,
});

// What a failing check means, said the way a household would. The check's own detail is for
// Diagnostics: it can name a setting key or a probe ("configured but unavailable").
const FAILED_SENTENCE: Record<string, string> = {
  media_server: "Your media server isn't answering.",
  tunarr: "Tunarr isn't answering.",
  requester: "A download service isn't answering.",
  tmdb: "TMDB isn't answering.",
  llm: "The AI service isn't answering.",
  public_url: "This server can't reach its own web address.",
  playout_tonemap: "HDR shows can't be converted for ordinary screens.",
  database: "The database isn't answering.",
};

const healthProblem = (check: HealthCheck): SettingsProblem | undefined => {
  // Overdue tasks are counted from the task list itself, which says how many.
  if (check.key === "scheduled_tasks") return undefined;
  const destinationId =
    SETTINGS_DESTINATIONS.find((item) => item.path === check.remediationRoute)?.id ?? "diagnostics";
  if (check.status === "skipped" && check.key === "media_server") {
    return problem(destinationId, "notSetUp", "No media server is connected yet.");
  }
  if (check.status === "stale") {
    return problem(destinationId, "attention", `${check.label} hasn't been checked for a while.`);
  }
  if (check.status === "failed" || check.status === "warning") {
    return problem(destinationId, "attention", FAILED_SENTENCE[check.key] ?? `${check.label} needs a look.`);
  }
  return undefined;
};

const plural = (count: number, one: string, many: string) => (count === 1 ? one : many);

// Every problem the checks Diagnostics already runs can see, worst first. Each input is optional:
// a check that has not answered, or that this viewer cannot read, adds nothing.
const settingsProblems = (input: SettingsStatusInput): SettingsProblem[] => {
  const found: SettingsProblem[] = [];

  for (const check of input.health?.checks ?? []) {
    const p = healthProblem(check);
    if (p) found.push(p);
  }

  const overdue = (input.jobs ?? []).filter((job) => job.overdue && !job.running).length;
  if (overdue > 0) {
    found.push(
      problem(
        "tasks",
        "overdue",
        `${overdue} background ${plural(overdue, "task is", "tasks are")} overdue.`,
      ),
    );
  }

  if (input.backups?.supported) {
    const missed = missingNights(input.backups.backups ?? []);
    if (missed > 0) {
      found.push(
        problem("backup", "attention", `Backups missed ${missed} ${plural(missed, "night", "nights")}.`),
      );
    }
  }

  const storage = input.fillerStorage;
  if (storage?.state === "paused") {
    found.push(
      problem("filler.storage", "attention", "Clips have filled their space, so new ones are paused."),
    );
  } else if (storage?.state === "approaching" && storage.softBudgetBytes > 0) {
    const percent = Math.round((storage.managedBytes / storage.softBudgetBytes) * 100);
    found.push(problem("filler.storage", "attention", `Clips are using ${percent}% of their space.`));
  }

  if (input.notifications) {
    const enabled = input.notifications.filter((provider) => provider.enabled);
    const failing = enabled.find(
      (provider) =>
        provider.health?.lastFailureAt &&
        (!provider.health.lastSuccessAt || provider.health.lastFailureAt > provider.health.lastSuccessAt),
    );
    if (failing) {
      found.push(
        problem("notifications", "attention", `${failing.label} couldn't send the last notification.`),
      );
    } else if (enabled.length === 0) {
      found.push(problem("notifications", "notSetUp", "Nothing tells you when something goes wrong."));
    }
  }

  return found.sort((a, b) => SEVERITY[a.kind] - SEVERITY[b.kind]);
};

interface SettingsCardStatus {
  // Absent while a check behind the card is still loading.
  status?: StatusWordKind;
  problem?: SettingsProblem;
}

// The cards each check speaks for. Health can name any card, so every admin card waits for it.
const CARD_SOURCES: Record<Exclude<SettingsCardId, "advanced">, readonly (keyof SettingsStatusInput)[]> = {
  connections: ["health"],
  ai: ["health"],
  channels: ["health"],
  filler: ["health", "fillerStorage"],
  people: ["health"],
  notifications: ["notifications"],
  server: ["health", "jobs", "backups"],
};

const cardStatus = (
  card: Exclude<SettingsCardId, "advanced">,
  problems: readonly SettingsProblem[],
  settled: ReadonlySet<keyof SettingsStatusInput>,
): SettingsCardStatus => {
  if (!CARD_SOURCES[card].every((source) => settled.has(source))) return {};
  const first = problems.find((p) => p.card === card);
  return first ? { status: first.kind, problem: first } : { status: "ok" };
};

// useSettingsStatus reads the checks behind the Settings index cards through the generated client.
// Members read only their own notification destinations: every other check is an admin read.
const useSettingsStatus = (isAdmin: boolean) => {
  const admin = { enabled: isAdmin, retry: false } as const;
  const health = diagnosticsApi.useGetCurrentHealth({ query: admin });
  const jobs = jobsApi.useJobsList({ query: admin });
  const backups = systemApi.useSystemBackupsList({ query: admin });
  const readiness = fillerApi.useFillerReadiness({ query: admin });
  const notifications = notificationsApi.useNotificationProvidersList({ query: { retry: false } });

  const problems = settingsProblems({
    health: unwrap(health.data),
    jobs: unwrap(jobs.data)?.jobs,
    backups: unwrap(backups.data),
    fillerStorage: unwrap(readiness.data)?.storage,
    notifications: unwrap(notifications.data)?.providers,
  });
  // A check that failed to answer counts as settled: it adds no problem rather than holding the
  // card's status back forever.
  const settled = new Set<keyof SettingsStatusInput>(
    (
      [
        ["health", health.isFetched],
        ["jobs", jobs.isFetched],
        ["backups", backups.isFetched],
        ["fillerStorage", readiness.isFetched],
        ["notifications", notifications.isFetched],
      ] as const
    ).flatMap(([source, fetched]) => (fetched ? [source] : [])),
  );

  return (card: Exclude<SettingsCardId, "advanced">) => cardStatus(card, problems, settled);
};

export type { SettingsCardStatus, SettingsProblem, SettingsProblemKind, SettingsStatusInput };
export { cardStatus, settingsProblems, useSettingsStatus };
