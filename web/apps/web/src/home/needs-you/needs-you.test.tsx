import type { FillerStorageStatusDTO } from "@loomarr/api/models/fillerStorageStatusDTO";
import type { JobView } from "@loomarr/api/models/jobView";
import { describe, expect, it } from "vitest";
import { needsYouItems } from ".";

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

const storage = (state: FillerStorageStatusDTO["state"]): FillerStorageStatusDTO => ({
  automatic: true,
  availableBytes: 0,
  filesystemReservedBytes: 0,
  freeBytes: 0,
  hardReserveBytes: 0,
  managedBytes: 91,
  reservedBytes: 0,
  softBudgetBytes: 100,
  state,
  totalBytes: 1000,
});

describe("needsYouItems", () => {
  it("lists each count above zero, worded for its number, with its page", () => {
    expect(
      needsYouItems({
        approvals: 3,
        fillerChoices: 1,
        libraryTitles: 4,
        jobs: [
          job({ overdue: true }),
          job({ name: "scan", overdue: true }),
          job({ overdue: true, running: true }),
        ],
      }),
    ).toEqual([
      {
        id: "approvals",
        value: "3",
        label: "requests to approve",
        short: "to approve",
        to: "/requests/needs-you",
      },
      {
        id: "filler",
        value: "1",
        label: "filler choice to make",
        short: "filler choices",
        to: "/filler/manage",
      },
      {
        id: "library",
        value: "4",
        label: "library titles to look at",
        short: "in Library",
        to: "/library/needs",
      },
      { id: "tasks", value: "2", label: "tasks overdue", short: "overdue", to: "/settings/system/tasks" },
    ]);
  });

  it("shows filler space only once it runs short, as a warning", () => {
    expect(
      needsYouItems({
        approvals: 0,
        fillerChoices: 0,
        libraryTitles: 0,
        jobs: [],
        fillerStorage: storage("healthy"),
      }),
    ).toEqual([]);
    expect(
      needsYouItems({
        approvals: 0,
        fillerChoices: 0,
        libraryTitles: 0,
        jobs: [],
        fillerStorage: storage("approaching"),
      }),
    ).toEqual([
      {
        id: "quota",
        value: "91%",
        label: "of filler space used",
        short: "filler space",
        warn: true,
        to: "/filler/settings",
      },
    ]);
  });

  it("counts a task late past twice its interval, not one seconds late on its tick", () => {
    const secondsAgo = (s: number) => new Date(Date.now() - s * 1000).toISOString();
    const items = (jobs: JobView[]) =>
      needsYouItems({ approvals: 0, fillerChoices: 0, libraryTitles: 0, jobs });
    expect(items([job({ nextRun: secondsAgo(5), overdue: false })])).toEqual([]);
    expect(items([job({ nextRun: secondsAgo(3 * 86_400), overdue: true })])).toMatchObject([
      { id: "tasks", value: "1", label: "task overdue" },
    ]);
  });

  // #3411: Library's own Needs you was missing from Home's line. One title reads in the singular.
  it("counts what Library's Needs you holds, and leads to it", () => {
    expect(needsYouItems({ approvals: 0, fillerChoices: 0, libraryTitles: 1, jobs: [] })).toEqual([
      {
        id: "library",
        value: "1",
        label: "library title to look at",
        short: "in Library",
        to: "/library/needs",
      },
    ]);
  });

  it("is empty when nothing needs the admin", () => {
    expect(needsYouItems({ approvals: 0, fillerChoices: 0, libraryTitles: 0, jobs: [job({})] })).toEqual([]);
  });
});
