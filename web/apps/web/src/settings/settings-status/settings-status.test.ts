import type { HealthCheck } from "@loomarr/api/models/healthCheck";
import type { HealthReport } from "@loomarr/api/models/healthReport";
import type { JobView } from "@loomarr/api/models/jobView";
import type { NotificationProviderDTO } from "@loomarr/api/models/notificationProviderDTO";
import { describe, expect, it } from "vitest";
import { cardStatus, settingsProblems } from ".";

const health = (checks: HealthCheck[]): HealthReport => ({
  checks,
  generation: 1,
  generationId: "g",
  generationStartedAt: 1,
  processStartedAt: 1,
  state: "degraded",
  updatedAt: 2,
  version: "dev",
});

const check = (key: string, status: HealthCheck["status"], remediationRoute?: string): HealthCheck => ({
  key,
  label: key === "llm" ? "AI provider" : "Media server",
  mode: "continuous",
  required: false,
  status,
  remediationRoute,
});

const job = (overrides: Partial<JobView>): JobView => ({
  description: "",
  group: "backup",
  lastResult: "ok",
  name: "backup",
  paused: false,
  running: false,
  schedule: "0 3 * * *",
  scheduleKey: "backup.schedule",
  title: "Back up",
  ...overrides,
});

const provider = (overrides: Partial<NotificationProviderDTO>): NotificationProviderDTO => ({
  createdAt: "2026-11-01T00:00:00Z",
  enabled: true,
  events: [],
  id: "p1",
  label: "Household email",
  settings: [],
  type: "smtp",
  updatedAt: "2026-11-01T00:00:00Z",
  ...overrides,
});

const at = (day: number) => Date.UTC(2026, 10, day, 12) / 1000;

describe("settingsProblems", () => {
  it("files a failing connection under the card its fix lives on, in plain words", () => {
    const [problem] = settingsProblems({
      health: health([check("media_server", "failed", "/settings/connections")]),
    });
    expect(problem).toEqual({
      destinationId: "connections",
      card: "connections",
      kind: "attention",
      text: "Your media server isn't answering.",
    });
  });

  it("sends the AI check to the AI card", () => {
    const [problem] = settingsProblems({ health: health([check("llm", "warning", "/settings/ai")]) });
    expect(problem?.card).toBe("ai");
    expect(problem?.text).toBe("The AI service isn't answering.");
  });

  it("calls an unconnected media server not set up, and ignores other skipped checks", () => {
    const problems = settingsProblems({
      health: health([
        check("media_server", "skipped", "/settings/connections"),
        check("tmdb", "skipped", "/settings/connections"),
      ]),
    });
    expect(problems).toEqual([
      {
        destinationId: "connections",
        card: "connections",
        kind: "notSetUp",
        text: "No media server is connected yet.",
      },
    ]);
  });

  // #3403: the AI card must not read OK while no provider is configured.
  it("calls an unconfigured AI provider not set up, so the AI card is neutral rather than OK", () => {
    const input = { health: health([check("llm", "skipped", "/settings/ai")]) };
    const [problem] = settingsProblems(input);
    expect(problem).toMatchObject({ card: "ai", kind: "notSetUp", text: "No AI provider is set up yet." });
    expect(cardStatus("ai", settingsProblems(input), new Set(["health"])).status).toBe("notSetUp");
  });

  it("counts overdue tasks from the task list, not the health check", () => {
    const problems = settingsProblems({
      health: health([check("scheduled_tasks", "warning", "/settings/system/tasks")]),
      jobs: [
        job({ overdue: true }),
        job({ name: "scan", overdue: true }),
        job({ overdue: true, running: true }),
      ],
    });
    expect(problems).toEqual([
      { destinationId: "tasks", card: "server", kind: "overdue", text: "2 background tasks are overdue." },
    ]);
  });

  it("reports missed backup nights", () => {
    const [problem] = settingsProblems({
      backups: {
        backups: [at(7), at(4)].map((writtenAt) => ({ name: `${writtenAt}`, bytes: 1, writtenAt })),
        dir: "/backups",
        retain: 7,
        schedule: "0 3 * * *",
        supported: true,
      },
    });
    expect(problem).toMatchObject({
      destinationId: "backup",
      card: "server",
      text: "Backups missed 2 nights.",
    });
  });

  it("reports filler space as a share of the allowance, and a pause outright", () => {
    const storage = {
      automatic: true,
      availableBytes: 0,
      filesystemReservedBytes: 0,
      freeBytes: 0,
      hardReserveBytes: 0,
      managedBytes: 91,
      reservedBytes: 0,
      softBudgetBytes: 100,
      totalBytes: 1000,
    };
    expect(settingsProblems({ fillerStorage: { ...storage, state: "approaching" } })[0]).toMatchObject({
      destinationId: "filler.storage",
      card: "filler",
      text: "Clips are using 91% of their space.",
    });
    expect(settingsProblems({ fillerStorage: { ...storage, state: "paused" } })[0]?.text).toBe(
      "Clips have filled their space, so new ones are paused.",
    );
    expect(settingsProblems({ fillerStorage: { ...storage, state: "healthy" } })).toEqual([]);
  });

  it("says when nothing will send a notification, and when the last one failed", () => {
    expect(settingsProblems({ notifications: [] })[0]).toMatchObject({
      kind: "notSetUp",
      card: "notifications",
    });
    expect(settingsProblems({ notifications: [provider({ enabled: false })] })[0]?.kind).toBe("notSetUp");
    expect(
      settingsProblems({
        notifications: [
          provider({
            health: {
              lastFailureAt: "2026-11-08T10:00:00Z",
              lastSuccessAt: "2026-11-07T10:00:00Z",
              queuedCount: 0,
              terminalFailureCount: 1,
            },
          }),
        ],
      })[0]?.text,
    ).toBe("Household email couldn't send the last notification.");
    expect(settingsProblems({ notifications: [provider({})] })).toEqual([]);
  });

  it("puts the worst problem first", () => {
    const problems = settingsProblems({
      notifications: [],
      jobs: [job({ overdue: true })],
      health: health([check("media_server", "failed", "/settings/connections")]),
    });
    expect(problems.map((p) => p.kind)).toEqual(["attention", "overdue", "notSetUp"]);
  });
});

describe("cardStatus", () => {
  const problems = settingsProblems({ jobs: [job({ overdue: true })] });

  it("waits for every check behind the card", () => {
    expect(cardStatus("server", problems, new Set(["health", "jobs"]))).toEqual({});
  });

  it("states the card's worst problem, or OK", () => {
    const settled = new Set(["health", "jobs", "backups"] as const);
    expect(cardStatus("server", problems, settled)).toMatchObject({ status: "overdue" });
    expect(cardStatus("ai", problems, settled)).toEqual({ status: "ok" });
  });
});
