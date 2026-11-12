import * as settingsApi from "@loomarr/api/endpoints/settings";
import type { LocationDTO } from "@loomarr/api/models/locationDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { Action, Combobox, type ComboboxOption, Icon, icons, Stack } from "@loomarr/design-system";
import { keepPreviousData } from "@tanstack/react-query";
import { useEffect, useId, useState } from "react";
import type { LocationPickerProps } from "./location-picker.type";

const countryName = (country: string) => {
  if (!country) return "";
  try {
    return new Intl.DisplayNames(["en"], { type: "region" }).of(country) ?? country;
  } catch {
    return country;
  }
};

// One location control for setup, settings, and source-specific exceptions. Search result
// behavior and optional device/network detection stay behind this interface so every caller gets
// the same keyboard, loading, and error behavior.
const LocationPicker = ({
  value,
  onChange,
  allowDetection = false,
  emptyHint = "Choose a location before continuing.",
  locked = false,
  lockedLabel,
  error,
  restrictCountry,
}: LocationPickerProps) => {
  const listID = useId();
  const place = value.place ?? "";
  const [evidence, setEvidence] = useState<LocationDTO>();
  // A saved place is an opaque id; its label is fetched. A place just picked here already
  // carries its label, and a country-only value labels itself.
  const pickedLabel = evidence && (evidence.place ?? "") === place ? evidence.label : undefined;
  const savedPlace = settingsApi.useLocationsGet(place, {
    query: { enabled: Boolean(place) && pickedLabel === undefined, retry: false },
  });
  const savedLabel = unwrap(savedPlace.data, (body) => body.location)?.label;
  const countryLabel = countryName(value.country);
  const selectedLabel = place
    ? (pickedLabel ?? savedLabel ?? (savedPlace.isError ? countryLabel : ""))
    : countryLabel;
  const [query, setQuery] = useState(selectedLabel);
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [detecting, setDetecting] = useState(false);
  const [locationError, setLocationError] = useState<string>();

  useEffect(() => setQuery(selectedLabel), [selectedLabel]);

  useEffect(() => {
    const trimmed = query.trim();
    if (trimmed.length < 2 || query === selectedLabel) {
      setDebouncedQuery("");
      return;
    }
    const timer = window.setTimeout(() => setDebouncedQuery(trimmed), 250);
    return () => window.clearTimeout(timer);
  }, [query, selectedLabel]);

  const search = settingsApi.useLocationsSearch(
    { q: debouncedQuery, limit: 8 },
    {
      query: {
        enabled: !locked && debouncedQuery.length >= 2,
        placeholderData: keepPreviousData,
        retry: false,
      },
    },
  );
  const found = unwrap(search.data, (body) => body.locations) ?? [];
  const locations = restrictCountry
    ? found.filter((candidate) => candidate.country === restrictCountry)
    : found;
  const trimmedQuery = query.trim();
  const showResults = !locked && trimmedQuery.length >= 2 && query !== selectedLabel;
  const waitingForQuery = debouncedQuery !== trimmedQuery;
  const choicesReady = !waitingForQuery && !search.isPlaceholderData;
  const searching = waitingForQuery || search.isFetching;
  const resolve = settingsApi.useLocationsResolve();
  // The city leads, and the region and country tell same-named cities apart.
  const options: ComboboxOption[] = locations.map((candidate) => ({
    description: candidate.city
      ? [candidate.region, countryName(candidate.country)].filter(Boolean).join(", ")
      : undefined,
    label: candidate.city || candidate.label,
    value: candidate.id,
  }));
  const chosen =
    locations.find((candidate) => candidate.country === value.country && (candidate.place ?? "") === place)
      ?.id ?? null;

  const choose = (picked: LocationDTO) => {
    setQuery(picked.label);
    setEvidence(picked);
    setLocationError(undefined);
    onChange(picked);
  };

  const approximateSuggestion = async () => {
    const response = await settingsApi.locationsSuggestion();
    const suggestion = unwrap(response, (body) => body.suggestion);
    if (!suggestion) return false;
    choose(suggestion);
    return true;
  };

  const detect = async () => {
    setDetecting(true);
    setLocationError(undefined);
    try {
      if (navigator.geolocation) {
        try {
          const position = await new Promise<GeolocationPosition>((resolvePosition, reject) =>
            navigator.geolocation.getCurrentPosition(resolvePosition, reject, {
              enableHighAccuracy: false,
              timeout: 8_000,
              maximumAge: 300_000,
            }),
          );
          const response = await resolve.mutateAsync({
            data: { latitude: position.coords.latitude, longitude: position.coords.longitude },
          });
          const located = unwrap(response, (body) => body.location);
          if (located) {
            choose(located);
            return;
          }
        } catch {
          // Permission denial and unsupported coordinates continue to the approximate fallback.
        }
      }
      if (!(await approximateSuggestion())) {
        setLocationError("Loomarr couldn't find your location. Search for your city or country instead.");
      }
    } catch {
      setLocationError("Loomarr couldn't find your location. Search for your city or country instead.");
    } finally {
      setDetecting(false);
    }
  };

  const visibleError = locationError ?? error;

  return (
    <div className="max-w-2xl">
      <Stack align="center" className="mb-1.5" direction="row" gap="inline">
        <label htmlFor={`${listID}-input`} className="text-sm font-medium">
          Location
        </label>
        {locked && lockedLabel && (
          <span className="bg-static-800 text-2xs text-static-400 inline-flex items-center gap-1 rounded-sm px-1.5 py-0.5 font-mono">
            <Icon decorative glyph={icons.locked} size="compact" tone="inherit" /> {lockedLabel}
          </span>
        )}
      </Stack>
      <div className="flex flex-col gap-2 sm:flex-row">
        <div className="min-w-0 flex-1">
          <Combobox
            autoHighlight
            disabled={locked}
            emptyText={
              showResults && !searching ? "No locations found. Try a nearby city or country." : undefined
            }
            errorText={
              showResults && !searching && search.isError
                ? "Location search is unavailable. Try again."
                : undefined
            }
            id={`${listID}-input`}
            inputValue={query}
            label="Location"
            loading={showResults && searching}
            onInputValueChange={(text) => {
              setQuery(text);
              setEvidence(undefined);
            }}
            onSelect={(option) => {
              const picked = locations.find((candidate) => candidate.id === option.value);
              if (picked) choose(picked);
            }}
            options={showResults ? options : []}
            placeholder="Search for a city or country"
            stale={!choicesReady}
            value={chosen}
          />
        </div>
        {allowDetection && (
          <Action
            disabled={locked || detecting}
            icon={detecting ? "loading" : undefined}
            onPress={() => void detect()}
            variant="outline"
          >
            {detecting ? "Finding…" : "Use my location"}
          </Action>
        )}
      </div>
      {evidence?.approximate && (
        <p className="text-muted-foreground mt-2 text-sm">
          Approximate location from your network. You can choose a more specific place above.
          {evidence.attribution && (
            <>
              {" "}
              <a className="underline" href={evidence.attribution} target="_blank" rel="noreferrer">
                IP Geolocation by DB-IP
              </a>
            </>
          )}
        </p>
      )}
      {visibleError && (
        <p className="text-destructive mt-2 text-sm" role="alert">
          {visibleError}
        </p>
      )}
      {!value.country && !visibleError && <p className="text-muted-foreground mt-2 text-sm">{emptyHint}</p>}
    </div>
  );
};

export { LocationPicker };
