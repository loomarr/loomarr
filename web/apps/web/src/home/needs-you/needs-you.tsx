import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as jobsApi from "@loomarr/api/endpoints/jobs";
import type { FillerStorageStatusDTO } from "@loomarr/api/models/fillerStorageStatusDTO";
import type { JobView } from "@loomarr/api/models/jobView";
import { unwrap } from "@loomarr/api/unwrap";
import type { NeedsLineItem } from "@loomarr/ui";
import { Link } from "@tanstack/react-router";
import { usePendingApprovals } from "@/queue/pending-approvals";

// Where each count leads: today's pages, no new routes.
type NeedTo = "/requests/needs-you" | "/filler/manage" | "/settings/system/tasks" | "/filler/settings";

interface NeedsYouItem {
  id: "approvals" | "filler" | "tasks" | "quota";
  value: string;
  label: string;
  short: string;
  warn?: boolean;
  to: NeedTo;
}

interface NeedsYouInput {
  // Submitted proposals and pending filler pulls: the Requests → Needs you approvals.
  approvals: number;
  // Incoming clips waiting on a choice: Filler → Manage's "Need a choice".
  fillerChoices: number;
  jobs: readonly JobView[];
  fillerStorage?: FillerStorageStatusDTO;
}

const count = (id: NeedsYouItem["id"], n: number, one: string, many: string, short: string, to: NeedTo) =>
  n > 0 ? [{ id, value: String(n), label: n === 1 ? one : many, short, to }] : [];

// The counts above zero, in the mock's order. Filler space is a warning, not a to-do: it shows only
// once space runs short, as the share of the allowance in use.
const needsYouItems = ({ approvals, fillerChoices, jobs, fillerStorage }: NeedsYouInput): NeedsYouItem[] => {
  const overdue = jobs.filter((job) => job.overdue && !job.running).length;
  const short =
    (fillerStorage?.state === "approaching" || fillerStorage?.state === "paused") &&
    fillerStorage.softBudgetBytes > 0;
  return [
    ...count(
      "approvals",
      approvals,
      "request to approve",
      "requests to approve",
      "to approve",
      "/requests/needs-you",
    ),
    ...count(
      "filler",
      fillerChoices,
      "filler choice to make",
      "filler choices to make",
      "filler choices",
      "/filler/manage",
    ),
    ...count("tasks", overdue, "task overdue", "tasks overdue", "overdue", "/settings/system/tasks"),
    ...(short && fillerStorage
      ? [
          {
            id: "quota" as const,
            value: `${Math.round((fillerStorage.managedBytes / fillerStorage.softBudgetBytes) * 100)}%`,
            label: "of filler space used",
            short: "filler space",
            warn: true,
            to: "/filler/settings" as const,
          },
        ]
      : []),
  ];
};

// useNeedsYou — the items of Home's "Needs you" line (#2785, layout B), each with its page as a
// router link. Every read is an admin read, so a member's Home fires none of them.
const useNeedsYou = (isAdmin: boolean): NeedsLineItem[] => {
  const admin = { enabled: isAdmin, retry: false } as const;
  const approvals = usePendingApprovals(isAdmin);
  const watch = unwrap(fillerApi.useFillerWatch({ query: admin }).data);
  const jobs = unwrap(jobsApi.useJobsList({ query: admin }).data)?.jobs ?? [];
  const readiness = unwrap(fillerApi.useFillerReadiness({ query: admin }).data);
  if (!isAdmin) return [];

  return needsYouItems({
    approvals: approvals.count,
    fillerChoices: watch?.needsYou ?? 0,
    jobs,
    fillerStorage: readiness?.storage,
  }).map(({ to, ...item }) => ({ ...item, link: <Link to={to} /> }));
};

export type { NeedsYouInput, NeedsYouItem };
export { needsYouItems, useNeedsYou };
