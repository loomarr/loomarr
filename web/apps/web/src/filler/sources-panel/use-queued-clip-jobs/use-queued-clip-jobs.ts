import * as fillerApi from "@loomarr/api/endpoints/filler";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueries, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

type QueuedClipJob = { sourceID: string; clipID: string; jobID: string };
type QueueStatus = Record<string, "queued" | "running" | "success" | "error">;

const QUEUED_CLIP_JOBS_KEY = "loomarr:source-item-jobs";

const readQueuedClipJobs = (): QueuedClipJob[] => {
  try {
    const parsed = JSON.parse(sessionStorage.getItem(QUEUED_CLIP_JOBS_KEY) ?? "[]");
    if (!Array.isArray(parsed)) return [];
    return parsed
      .filter(
        (item): item is QueuedClipJob =>
          typeof item?.sourceID === "string" &&
          typeof item?.clipID === "string" &&
          typeof item?.jobID === "string",
      )
      .slice(-50);
  } catch {
    return [];
  }
};

const rememberQueuedClipJobs = (jobs: QueuedClipJob[]): void => {
  try {
    sessionStorage.setItem(QUEUED_CLIP_JOBS_KEY, JSON.stringify(jobs.slice(-50)));
  } catch {
    // Browser storage is only correlation; the acquisition resource remains authoritative.
  }
};

// Queueing a remote clip from a source and following it to completion. Keeps only the browser's
// item→job correlation (in session storage); status itself is always read from the durable
// acquisition resource, so a dropped SSE frame costs latency rather than truth.
const useQueuedClipJobs = (sourceID: string) => {
  const queryClient = useQueryClient();
  const [queuedJobs, setQueuedJobs] = useState<QueuedClipJob[]>(readQueuedClipJobs);
  const [queueingId, setQueueingId] = useState<string>();
  const sourceJobs = queuedJobs.filter((job) => job.sourceID === sourceID);
  const acquisitionQueries = useQueries({
    queries: sourceJobs.map((job) => {
      const cached = queryClient.getQueryData<fillerApi.GetFillerAcquisitionQueryResult>(
        fillerApi.getGetFillerAcquisitionQueryKey(job.jobID),
      );
      const run = unwrap(cached, (body) => body);
      return {
        ...fillerApi.getGetFillerAcquisitionQueryOptions(job.jobID),
        refetchInterval: run && (run.status === "success" || run.status === "error") ? false : 1_500,
      };
    }),
  });
  const queueStatus = Object.fromEntries(
    sourceJobs
      .map((job, index) => ({ job, query: acquisitionQueries[index] }))
      .map(({ job, query }) => {
        const run = unwrap(query?.data, (body) => body);
        return [job.clipID, query?.isError ? "error" : (run?.status ?? "queued")];
      }),
  ) as QueueStatus;
  const queueClip = fillerApi.useQueueFillerSourceItem({
    mutation: {
      onSettled: () => setQueueingId(undefined),
      onSuccess: (response, variables) => {
        const result = unwrap(response, (body) => body);
        if (result?.jobId) {
          setQueuedJobs((previous) => {
            const next = [
              ...previous.filter(
                (job) => job.sourceID !== variables.id || job.clipID !== variables.data.remoteId,
              ),
              { sourceID: variables.id, clipID: variables.data.remoteId, jobID: result.jobId },
            ].slice(-50);
            rememberQueuedClipJobs(next);
            return next;
          });
        }
      },
    },
  });

  return {
    queueStatus,
    queue: (clip: { id: string; url: string }) => {
      setQueueingId(clip.id);
      queueClip.mutate({ id: sourceID, data: { remoteId: clip.id, url: clip.url } });
    },
    /** The remote id being queued right now, if any. */
    queueing: queueClip.isPending ? queueingId : undefined,
    error: queueClip.error?.detail,
  };
};

export type { QueuedClipJob };
export { QUEUED_CLIP_JOBS_KEY, readQueuedClipJobs, useQueuedClipJobs };
