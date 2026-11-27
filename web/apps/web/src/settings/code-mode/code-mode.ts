import { customFetch } from "@loomarr/api/mutator";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

// Code Mode (#4486): an admin lets an AI agent paired as one of their devices run scripts against the
// whole API. These read and set which of the admin's devices have it on.
//
// The routes are hidden from the OpenAPI document, like the rest of the agents surface (the device
// list's own DTO cannot carry the flag yet), so there are no generated hooks; customFetch is the
// same transport the generated hooks use, with CSRF, credentials and typed ApiError.

const codeModeKey = ["agents", "code-mode"] as const;

// `enabled` is false for anyone but an admin: the routes refuse members, and only an admin's devices
// can use Code Mode.
const useCodeModeDevices = (enabled: boolean) =>
  useQuery({
    queryKey: codeModeKey,
    enabled,
    queryFn: async () => {
      const res = await customFetch<{ data: { devices: string[] } }>("/v1/agents/code-mode");
      return new Set(res.data.devices);
    },
  });

const useSetCodeMode = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, on }: { id: string; on: boolean }) =>
      customFetch(`/v1/agents/code-mode/${encodeURIComponent(id)}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ on }),
      }),
    onSettled: () => queryClient.invalidateQueries({ queryKey: codeModeKey }),
  });
};

export { useCodeModeDevices, useSetCodeMode };
