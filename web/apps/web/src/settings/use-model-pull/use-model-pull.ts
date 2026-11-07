import * as systemApi from "@loomarr/api/endpoints/system";
import type { SystemLLMPullOperationDTO } from "@loomarr/api/models/systemLLMPullOperationDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

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

// One Ollama download, read from the operation that POST /pull returns. The operation is the
// only state: progress and failure are derived from it, and polling stops once it ends.
// `llm_pull` SSE frames invalidate every /v1/system/llm query (core events), so a frame only
// makes the next poll come sooner and a dropped frame loses nothing.
const useModelPull = (): ModelPull => {
  const queryClient = useQueryClient();
  const [jobId, setJobId] = useState<string>();
  const pull = systemApi.useSystemLlmPull({
    mutation: { onSuccess: (response) => setJobId(unwrap(response)?.jobId) },
  });
  const query = systemApi.useSystemLlmPullOperation(jobId ?? "", {
    query: {
      enabled: Boolean(jobId),
      retry: false,
      refetchInterval: (current) => (isTerminal(unwrap(current.state.data)) ? false : 1_000),
    },
  });
  const operation = jobId ? unwrap(query.data) : undefined;
  const status = operation?.jobId === jobId ? operation?.status : undefined;

  // A finished download changes what is installed and what is left to download.
  useEffect(() => {
    if (status !== "success") return;
    void queryClient.invalidateQueries({ queryKey: systemApi.getSystemLlmStatusQueryKey() });
    void queryClient.invalidateQueries({ queryKey: systemApi.getSystemLlmDiscoverQueryKey() });
  }, [status, queryClient]);

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
