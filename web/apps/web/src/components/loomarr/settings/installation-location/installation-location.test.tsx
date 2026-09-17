import type { LocationDTO, SettingEntry } from "@loomarr/api";
import { SettingEntryApply } from "@loomarr/api/models/settingEntryApply";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { InstallationLocation } from "./installation-location";

const mocks = vi.hoisted(() => ({
  locations: [] as LocationDTO[],
  search: vi.fn(),
  resolve: vi.fn(),
  suggestion: vi.fn(),
}));

vi.mock(import("@loomarr/api/endpoints/settings"), async (importOriginal) => {
  const actual = await importOriginal();
  return {
    ...actual,
    useLocationsSearch: ((...args: unknown[]) => {
      mocks.search(...args);
      return {
        data: { status: 200, data: { locations: mocks.locations } },
        error: null,
        isFetching: false,
      };
    }) as unknown as typeof actual.useLocationsSearch,
    useLocationsResolve: (() => ({
      mutateAsync: mocks.resolve,
      isPending: false,
    })) as unknown as typeof actual.useLocationsResolve,
    locationsSuggestion: mocks.suggestion as unknown as typeof actual.locationsSuggestion,
  };
});

const entry = (key: string, value = ""): SettingEntry => ({
  key,
  label: key.endsWith("country") ? "Country" : "Local area",
  group: "filler",
  owner: "settings.location",
  kind: "string",
  value,
  provenance: "default",
  apply: SettingEntryApply.live,
  advanced: false,
  doc: "",
  secret: false,
  set: value !== "",
});

const entries = () => [entry("filler.home_country"), entry("filler.home_market")];

describe("InstallationLocation", () => {
  beforeEach(() => {
    mocks.locations = [];
    mocks.search.mockReset();
    mocks.resolve.mockReset();
    mocks.suggestion.mockReset();
    Object.defineProperty(navigator, "geolocation", { configurable: true, value: undefined });
  });

  it("edits country and market through one searchable Location field", async () => {
    mocks.locations = [
      {
        id: "5128581",
        label: "New York City, United States",
        country: "US",
        market: "New York City",
        region: "New York",
      },
    ];
    const onChange = vi.fn();
    render(<InstallationLocation entries={entries()} values={{}} onChange={onChange} />);

    expect(screen.getAllByRole("combobox")).toHaveLength(1);
    expect(screen.queryByRole("textbox", { name: "Country" })).not.toBeInTheDocument();
    await userEvent.type(screen.getByRole("combobox", { name: "Location" }), "New York");
    const option = screen.getByRole("option", { name: "New York City, United States" });
    await waitFor(() => expect(option).toBeEnabled());
    expect(option).toHaveTextContent("New York, United States");
    await userEvent.click(option);

    expect(onChange).toHaveBeenNthCalledWith(1, "filler.home_country", "US");
    expect(onChange).toHaveBeenNthCalledWith(2, "filler.home_market", "New York City");
  });

  it("waits for a typing pause before searching", async () => {
    render(<InstallationLocation entries={entries()} values={{}} onChange={vi.fn()} />);

    const input = screen.getByRole("combobox", { name: "Location" });
    await userEvent.type(input, "North Greenbush");

    const queriesBeforePause = mocks.search.mock.calls.map((call) => call[0]?.q);
    expect(queriesBeforePause.filter(Boolean)).toEqual([]);

    await waitFor(() => {
      const queriesAfterPause = mocks.search.mock.calls.map((call) => call[0]?.q).filter(Boolean);
      expect(queriesAfterPause).toEqual(["North Greenbush"]);
    });
  });

  it("uses browser coordinates even when the browser language belongs to another country", async () => {
    Object.defineProperty(navigator, "languages", { configurable: true, value: ["en-GB"] });
    Object.defineProperty(navigator, "geolocation", {
      configurable: true,
      value: {
        getCurrentPosition: (success: PositionCallback) =>
          success({ coords: { latitude: 40.7128, longitude: -74.006 } } as GeolocationPosition),
      },
    });
    mocks.resolve.mockResolvedValue({
      status: 200,
      data: {
        location: {
          id: "5128581",
          label: "New York City, United States",
          country: "US",
          market: "New York City",
          source: "device",
        },
      },
    });
    const onChange = vi.fn();
    render(<InstallationLocation entries={entries()} values={{}} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: "Use my location" }));

    expect(mocks.resolve).toHaveBeenCalledWith({ data: { latitude: 40.7128, longitude: -74.006 } });
    expect(onChange).toHaveBeenCalledWith("filler.home_country", "US");
    expect(onChange).not.toHaveBeenCalledWith("filler.home_country", "GB");
  });

  it("falls back to a trusted proxy or local-IP suggestion when device location is unavailable", async () => {
    mocks.suggestion.mockResolvedValue({
      status: 200,
      data: {
        suggestion: {
          id: "country-CA",
          label: "Canada",
          country: "CA",
          source: "cloudflare",
          approximate: true,
        },
      },
    });
    const onChange = vi.fn();
    render(<InstallationLocation entries={entries()} values={{}} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: "Use my location" }));

    expect(onChange).toHaveBeenCalledWith("filler.home_country", "CA");
    expect(onChange).toHaveBeenCalledWith("filler.home_market", "");
    expect(screen.getByText(/approximate/i)).toBeInTheDocument();
  });

  it("keeps manual search usable when automatic location cannot find an answer", async () => {
    mocks.suggestion.mockResolvedValue({ status: 200, data: {} });
    render(<InstallationLocation entries={entries()} values={{}} onChange={vi.fn()} />);

    await userEvent.click(screen.getByRole("button", { name: "Use my location" }));

    expect(screen.getByRole("alert")).toHaveTextContent("Search for your city or country instead");
    expect(screen.getByRole("combobox", { name: "Location" })).toBeEnabled();
  });

  it("does not offer to replace environment-pinned geography", () => {
    render(
      <InstallationLocation
        entries={[
          { ...entry("filler.home_country", "US"), provenance: "env", envVar: "FILLER_HOME_COUNTRY" },
          entry("filler.home_market", "New York City"),
        ]}
        values={{}}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getByRole("combobox", { name: "Location" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Use my location" })).toBeDisabled();
  });
});
