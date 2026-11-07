import type { LocationDTO } from "@loomarr/api";
import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { usePlaceLabel } from "./use-place-label";

const mocks = vi.hoisted(() => ({
  locations: [] as LocationDTO[],
  get: vi.fn(),
}));

vi.mock(import("@loomarr/api/endpoints/settings"), async (importOriginal) => {
  const actual = await importOriginal();
  return {
    ...actual,
    useLocationsGet: ((id: string, options: unknown) => {
      mocks.get(id, options);
      const location = mocks.locations.find((candidate) => candidate.place === id);
      return { data: location ? { status: 200, data: { location } } : undefined };
    }) as unknown as typeof actual.useLocationsGet,
  };
});

describe("usePlaceLabel", () => {
  beforeEach(() => {
    mocks.get.mockClear();
    mocks.locations = [{ id: "harbor", place: "harbor", country: "US", label: "Harbor City, ST" }];
  });

  it("shows the label the operator picked, never the saved id", () => {
    const { result } = renderHook(() => usePlaceLabel("harbor"));
    expect(result.current).toBe("Harbor City, ST");
  });

  it("asks for nothing when there is no place", () => {
    const { result } = renderHook(() => usePlaceLabel(undefined));
    expect(result.current).toBeUndefined();
    expect(mocks.get).toHaveBeenCalledWith("", { query: { enabled: false, retry: false } });
  });

  it("shows nothing for a place the index no longer holds", () => {
    const { result } = renderHook(() => usePlaceLabel("gone"));
    expect(result.current).toBeUndefined();
  });
});
