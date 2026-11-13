import type { JobView } from "@loomarr/api/models/jobView";
import { describe, expect, it } from "vitest";
import { isOverdueTask, overdueTaskCount } from ".";

const job = (overrides: Partial<JobView>): JobView => ({
  description: "",
  group: "backup",
  lastResult: "ok",
  name: "backup",
  paused: false,
  running: false,
  schedule: "0 30 3 * * *",
  scheduleKey: "backup.schedule",
  title: "Back up",
  ...overrides,
});

const secondsAgo = (s: number) => new Date(Date.now() - s * 1000).toISOString();

describe("overdueTaskCount", () => {
  it("does not count a task that is seconds late on its tick", () => {
    // The server flags overdue only past twice the task's interval; five seconds late is queueing.
    expect(overdueTaskCount([job({ nextRun: secondsAgo(5), overdue: false })])).toBe(0);
  });

  it("counts a task late by more than twice its interval", () => {
    expect(overdueTaskCount([job({ nextRun: secondsAgo(3 * 86_400), overdue: true })])).toBe(1);
  });

  it("does not count a running task", () => {
    expect(isOverdueTask(job({ overdue: true, running: true }))).toBe(false);
  });
});
