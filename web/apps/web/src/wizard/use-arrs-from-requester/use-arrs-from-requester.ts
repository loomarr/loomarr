import * as setupApi from "@loomarr/api/endpoints/setup";
import type { ArrFromRequesterDTO } from "@loomarr/api/models/arrFromRequesterDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { invalidateSettings } from "@loomarr/core/events";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";

interface UseArrsFromRequesterOptions {
  // Sonarr or Radarr has no address yet: ask Seerr once.
  missing: boolean;
  // Called after Seerr filled one in, so the step can look for the folders it names.
  onFilled?: () => void;
}

// useArrsFromRequester fills in Sonarr and Radarr from the ones Seerr uses, once, when either is
// missing (#2151). The server saves only an arr that has no address, so asking never overwrites
// what the operator typed.
const useArrsFromRequester = ({ missing, onFilled }: UseArrsFromRequesterOptions) => {
  const queryClient = useQueryClient();
  const fill = setupApi.useArrsFromRequester({
    mutation: {
      onSuccess: async (response) => {
        await invalidateSettings(queryClient);
        if (unwrap(response, (b) => b.arrs)?.some((a) => a.filled)) onFilled?.();
      },
    },
  });
  const asked = useRef(false);
  const { mutate } = fill;
  useEffect(() => {
    if (!missing || asked.current) return;
    asked.current = true;
    mutate();
  }, [missing, mutate]);
  const arrs: ArrFromRequesterDTO[] = unwrap(fill.data, (b) => b.arrs) ?? [];
  return { filled: arrs.filter((a) => a.filled), error: fill.error };
};

export { useArrsFromRequester };
