import type { JobView } from "@loomarr/api/models/jobView";

// The one reading of "overdue" a person is told. The server sets `overdue` only when a task is past
// due by more than twice its own interval (#3402), so a task seconds late on its tick is ordinary
// queueing and never counts. A running task is not waiting on anything.
const isOverdueTask = (job: JobView): boolean => Boolean(job.overdue) && !job.running;

// overdueTaskCount is the number Home's Needs you line, the Settings index and each Tasks group
// summary all say, so the three can never disagree.
const overdueTaskCount = (jobs: readonly JobView[]): number => jobs.filter(isOverdueTask).length;

export { isOverdueTask, overdueTaskCount };
