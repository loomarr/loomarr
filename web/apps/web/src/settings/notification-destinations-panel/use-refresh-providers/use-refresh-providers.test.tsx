import * as notificationsApi from "@loomarr/api/endpoints/notifications";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { useRefreshProviders } from "./use-refresh-providers";

describe("useRefreshProviders", () => {
  it("invalidates the provider list and nothing else", async () => {
    const client = new QueryClient();
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useRefreshProviders(), { wrapper });

    await result.current();

    expect(invalidate).toHaveBeenCalledTimes(1);
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: notificationsApi.getNotificationProvidersListQueryKey(),
    });
  });
});
