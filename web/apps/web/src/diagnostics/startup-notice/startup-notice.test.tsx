import { getGetCurrentHealthMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { HealthNotice } from "./startup-notice";

const mocks = vi.hoisted(() => ({
  navigate: vi.fn(),
  success: vi.fn(
    (_message: string, _options: { duration: number; onAutoClose?: () => void; onDismiss?: () => void }) =>
      "healthy-toast",
  ),
  warning: vi.fn(
    (_message: string, _options: { duration: number; onAutoClose?: () => void; onDismiss?: () => void }) =>
      "warning-toast",
  ),
  error: vi.fn(
    (_message: string, _options: { duration: number; onAutoClose?: () => void; onDismiss?: () => void }) =>
      "error-toast",
  ),
  dismiss: vi.fn(),
}));

vi.mock("@tanstack/react-router", () => ({ useNavigate: () => mocks.navigate }));
vi.mock("sonner", () => ({
  toast: { success: mocks.success, warning: mocks.warning, error: mocks.error, dismiss: mocks.dismiss },
}));

const report = (state: "healthy" | "degraded" | "unhealthy", updatedAt = 2) => ({
  generationId: "startup-1",
  generation: 1,
  version: "v1.2.3",
  processStartedAt: 1,
  generationStartedAt: 1,
  updatedAt,
  state,
  checks:
    state === "healthy"
      ? []
      : [
          {
            key: "media_server",
            label: "Media server",
            required: false,
            mode: "continuous" as const,
            status: "warning" as const,
            detail: "Configured but unavailable",
          },
        ],
});

const renderNotice = (enabled = true) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return {
    client,
    view: render(
      <QueryClientProvider client={client}>
        <HealthNotice enabled={enabled} />
      </QueryClientProvider>,
    ),
  };
};

describe("HealthNotice", () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => vi.clearAllMocks());

  it("shows a calm healthy notice once and records automatic acknowledgement", async () => {
    server.use(getGetCurrentHealthMockHandler(report("healthy")));
    renderNotice();
    await waitFor(() => expect(mocks.success).toHaveBeenCalledOnce());
    const options = mocks.success.mock.calls[0]?.[1];
    expect(options?.duration).toBe(6_000);
    act(() => options?.onAutoClose?.());
    expect(localStorage.getItem("loomarr.health.ack.startup-1:1:healthy:")).toBe("1");
  });

  it("keeps a degraded incident visible until the operator acknowledges it", async () => {
    server.use(getGetCurrentHealthMockHandler(report("degraded")));
    renderNotice();
    await waitFor(() => expect(mocks.warning).toHaveBeenCalledOnce());
    const options = mocks.warning.mock.calls[0]?.[1];
    expect(options?.duration).toBe(Number.POSITIVE_INFINITY);
    const key = "loomarr.health.ack.startup-1:1:degraded:media_server:warning";
    expect(localStorage.getItem(key)).toBeNull();
    act(() => options?.onDismiss?.());
    expect(localStorage.getItem(key)).toBe("1");
  });

  it("does not replace an unchanged persistent incident after a fresh poll", async () => {
    server.use(getGetCurrentHealthMockHandler(report("degraded", 2)));
    const { client } = renderNotice();
    await waitFor(() => expect(mocks.warning).toHaveBeenCalledOnce());

    server.use(getGetCurrentHealthMockHandler(report("degraded", 3)));
    await act(() => client.invalidateQueries());
    await waitFor(() => expect(client.isFetching()).toBe(0));
    expect(mocks.warning).toHaveBeenCalledOnce();
  });

  it("announces recovery once after a persistent incident", async () => {
    server.use(getGetCurrentHealthMockHandler(report("degraded")));
    const { client } = renderNotice();
    await waitFor(() => expect(mocks.warning).toHaveBeenCalledOnce());

    server.use(getGetCurrentHealthMockHandler(report("healthy", 3)));
    await act(() => client.invalidateQueries());
    await waitFor(() =>
      expect(mocks.success).toHaveBeenCalledWith("Loomarr health recovered", expect.anything()),
    );
  });

  it("does not query or notify for a non-admin shell", async () => {
    renderNotice(false);
    await act(async () => Promise.resolve());
    expect(mocks.success).not.toHaveBeenCalled();
    expect(mocks.warning).not.toHaveBeenCalled();
    expect(mocks.error).not.toHaveBeenCalled();
  });
});
