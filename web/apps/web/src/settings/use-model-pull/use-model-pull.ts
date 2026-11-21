import * as systemApi from "@loomarr/api/endpoints/system";
import type { SystemLLMPullOperationDTO } from "@loomarr/api/models/systemLLMPullOperationDTO";
import { unwrap } from "@loomarr/api/unwrap";
import type { LlmPullEvent } from "@loomarr/core/events";
import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useLoomarrEventListener } from "@/events/events-provider";

type ModelPullProgress = { tag: string; percent?: number };

type ModelPull = {
  // The model downloading right now, with its percent once the backend reports one.
  pulling?: ModelPullProgress;
  // Why the last download failed: the request itself, or the operation ending in error.
  error?: unknown;
  start: (model: string) => void;
};

const isTerminal = (operation?: SystemLLMPullOperationDTO) =>
  operation?.status === "success" || operation?.status === "error";

// A frame's status is Ollama's own words ("pulling manifest", "verifying sha256 digest") until the
// download ends in Loomarr's success or error; the operation only knows whether it still runs.
const operationStatus = (frame: LlmPullEvent): SystemLLMPullOperationDTO["status"] =>
  frame.status === "success" || frame.status === "error" ? frame.status : "running";

/** Writes one llm_pull frame into its download's cached operation, if that operation is cached. */
const applyPullFrame = (queryClient: QueryClient, frame: LlmPullEvent) =>
  queryClient.setQueryData<systemApi.systemLlmPullOperationResponse>(
    systemApi.getSystemLlmPullOperationQueryKey(frame.jobId),
    (old) =>
      old?.status === 200
        ? {
            ...old,
            data: {
              ...old.data,
              status: operationStatus(frame),
              percent: frame.percent,
              completed: frame.completed,
              total: frame.total,
              error: frame.error,
            },
          }
        : old,
  );

// One Ollama download, read from the operation that POST /pull returns. The operation is the
// only state: progress and failure are derived from it. Its GET is the truth when the download
// starts and on reconnect; each llm_pull frame is written into it while the download runs, and the
// bus refetches every /v1/system/llm read once a frame says the download ended (core events), so
// what is installed and what is left to download follow too.
const useModelPull = (): ModelPull => {
  const queryClient = useQueryClient();
  const [jobId, setJobId] = useState<string>();
  const pull = systemApi.useSystemLlmPull({
    mutation: { onSuccess: (response) => setJobId(unwrap(response)?.jobId) },
  });
  const query = systemApi.useSystemLlmPullOperation(jobId ?? "", {
    query: { enabled: Boolean(jobId), retry: false },
  });
  useLoomarrEventListener({ onLlmPull: (frame) => applyPullFrame(queryClient, frame) });
  const operation = jobId ? unwrap(query.data) : undefined;
  const status = operation?.jobId === jobId ? operation?.status : undefined;

  const requested = pull.variables?.data.model;
  let pulling: ModelPullProgress | undefined;
  if (pull.isPending && requested) {
    pulling = { tag: requested };
  } else if (jobId && !isTerminal(operation)) {
    const tag = operation?.model ?? requested;
    if (tag) pulling = { tag, percent: operation && operation.percent >= 0 ? operation.percent : undefined };
  }

  const error =
    pull.error ??
    (status === "error" && operation
      ? new Error(operation.error ?? `Downloading ${operation.model} failed.`)
      : undefined);

  const start = (model: string) => {
    setJobId(undefined);
    pull.mutate({ data: { model } });
  };

  return { pulling, error, start };
};

export type { ModelPull, ModelPullProgress };
export { useModelPull };
