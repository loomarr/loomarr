import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import { useCallback, useEffect, useState } from "react";

const keyFor = (jobId: string) => `loomarr.proposalReviewEdit.${jobId}`;

const readEdit = (jobId?: string): ApprovalEditDTO | undefined => {
  if (!jobId || typeof window === "undefined") return undefined;
  try {
    const value = JSON.parse(
      window.sessionStorage.getItem(keyFor(jobId)) ?? "null",
    ) as ApprovalEditDTO | null;
    return value && typeof value === "object" ? value : undefined;
  } catch {
    window.sessionStorage.removeItem(keyFor(jobId));
    return undefined;
  }
};

// Pending title choices are part of the resumable review, but never server state:
// this session-scoped delta follows the same durable Job id and still reaches the
// backend only as a parameter to the existing approval gate.
const useProposalReviewEdit = (jobId?: string) => {
  const [snapshot, setSnapshot] = useState<{ jobId?: string; edit?: ApprovalEditDTO }>(() => ({
    jobId,
    edit: readEdit(jobId),
  }));

  useEffect(() => {
    if (snapshot.jobId !== jobId) setSnapshot({ jobId, edit: readEdit(jobId) });
  }, [jobId, snapshot.jobId]);

  const setEdit = useCallback(
    (edit: ApprovalEditDTO | undefined) => {
      setSnapshot({ jobId, edit });
      if (!jobId || typeof window === "undefined") return;
      if (edit) window.sessionStorage.setItem(keyFor(jobId), JSON.stringify(edit));
      else window.sessionStorage.removeItem(keyFor(jobId));
    },
    [jobId],
  );

  return [snapshot.jobId === jobId ? snapshot.edit : undefined, setEdit] as const;
};

export { useProposalReviewEdit };
