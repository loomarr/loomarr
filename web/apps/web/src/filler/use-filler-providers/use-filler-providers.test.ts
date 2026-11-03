import { getListFillerProvidersMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { fillerProviders } from "@/test/fixtures/filler";
import { server } from "@/test/msw/server";
import { useFillerProviders } from "./use-filler-providers";

const wrapper = ({ children }: { children: ReactNode }) =>
  createElement(
    QueryClientProvider,
    { client: new QueryClient({ defaultOptions: { queries: { retry: false } } }) },
    children,
  );

// The web never names a provider: what it may offer for one comes from the registry the backend
// answers, so these read the answer rather than a list in the page.
describe("useFillerProviders", () => {
  it("answers nothing until the registry has loaded", () => {
    server.use(getListFillerProvidersMockHandler({ providers: fillerProviders }));
    const { result } = renderHook(() => useFillerProviders(), { wrapper });
    expect(result.current.find("archive")).toBeUndefined();
    expect(result.current.discoverable("archive")).toBeUndefined();
  });

  it("finds a provider by the id a source row carries", async () => {
    server.use(getListFillerProvidersMockHandler({ providers: fillerProviders }));
    const { result } = renderHook(() => useFillerProviders(), { wrapper });
    await waitFor(() => expect(result.current.find("youtube")?.label).toBe("YouTube"));
    expect(result.current.find("nonesuch")).toBeUndefined();
  });

  it("offers only the providers that can suggest and verify sources", async () => {
    server.use(getListFillerProvidersMockHandler({ providers: fillerProviders }));
    const { result } = renderHook(() => useFillerProviders(), { wrapper });
    await waitFor(() => expect(result.current.discoverable("archive")?.id).toBe("archive"));
    expect(result.current.discoverable("folder")).toBeUndefined();
    expect(result.current.discoverable("library")).toBeUndefined();
  });
});
