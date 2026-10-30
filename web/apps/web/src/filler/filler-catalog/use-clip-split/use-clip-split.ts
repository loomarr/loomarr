import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import { isOk } from "@loomarr/api/unwrap";
import { useNavigate } from "@tanstack/react-router";
import { useRef, useState } from "react";
import { useLoomarrEventListener } from "@/events/events-provider";

interface SplitJob {
  clipHash: string;
  // The status line reads better with a name than a content hash; carried alongside the
  // identity because the DTO's wire identity (hash) is meaningless to read in a sentence.
  clipName: string;
  jobId: string;
  status: string;
  error?: string;
}

// Splitting a compilation into its segments (§10 V34). The request returns a job id immediately and
// progress arrives on the event bus as `filler_split` frames; success NAVIGATES to the review route,
// so only the running and error states are ever visible here. Same shape as the ingest job.
const useClipSplit = () => {
  const navigate = useNavigate();
  const [job, setJob] = useState<SplitJob>();
  // The clip name for whatever split is currently in flight (§10 V45a) — a ref, not state, because it
  // is write-then-read-once inside the mutation callback below and never rendered itself; only the
  // resulting `job.clipName` is.
  const pendingName = useRef<string | undefined>(undefined);
  const split = fillerApi.useSplitFiller({
    mutation: {
      onSuccess: (res, vars) => {
        if (isOk(res)) {
          setJob({
            clipHash: vars.data.hash,
            clipName: pendingName.current ?? vars.data.hash,
            jobId: res.data.jobId,
            status: "running",
          });
        }
      },
    },
  });

  useLoomarrEventListener({
    onFillerSplit: (e) => {
      // Frames for OTHER split jobs (another tab, another admin) are not ours to act on.
      if (!e.jobId || e.jobId !== job?.jobId) return;
      if (e.status === "success" && e.proposalId) {
        setJob(undefined);
        void navigate({
          to: "/filler/splits/$proposalId",
          params: { proposalId: e.proposalId },
          search: { from: "library" },
        });
      } else if (e.status === "error") {
        setJob((prev) => (prev ? { ...prev, status: "error", error: e.error } : prev));
      }
    },
  });

  return {
    job,
    /** The request's own failure, distinct from a job that later reports an error. */
    error: split.error,
    start: (clip: ClipDTO) => {
      pendingName.current = clip.name;
      split.mutate({ data: { hash: clip.hash } });
    },
    /** Whether a split of this clip is running right now. */
    isRunning: (hash: string) => Boolean(job) && job?.clipHash === hash && job.status === "running",
  };
};

export { useClipSplit };
