import * as settingsApi from "@loomarr/api/endpoints/settings";
import type { LocationDTO } from "@loomarr/api/models/locationDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { keepPreviousData } from "@tanstack/react-query";
import { Check, Loader2, LocateFixed, Lock } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import type { LocationPickerProps } from "./location-picker.type";

const countryName = (country: string) => {
  if (!country) return "";
  try {
    return new Intl.DisplayNames(["en"], { type: "region" }).of(country) ?? country;
  } catch {
    return country;
  }
};

const locationLabel = (country: string, market: string) => {
  const name = countryName(country);
  return market ? `${market}, ${name || country}` : name || country;
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
}: LocationPickerProps) => {
  const listID = useId();
  const selectedLabel = locationLabel(value.country, value.market ?? "");
  const [query, setQuery] = useState(selectedLabel);
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const [detecting, setDetecting] = useState(false);
  const [locationError, setLocationError] = useState<string>();
  const [evidence, setEvidence] = useState<LocationDTO>();

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
        enabled: !locked && open && debouncedQuery.length >= 2,
        placeholderData: keepPreviousData,
        retry: false,
      },
    },
  );
  const locations = unwrap(search.data, (body) => body.locations) ?? [];
  const trimmedQuery = query.trim();
  const showResults = !locked && open && trimmedQuery.length >= 2 && query !== selectedLabel;
  const waitingForQuery = debouncedQuery !== trimmedQuery;
  const choicesReady = !waitingForQuery && !search.isPlaceholderData;
  const searching = waitingForQuery || search.isFetching;
  const resolve = settingsApi.useLocationsResolve();

  const choose = (place: LocationDTO) => {
    setQuery(place.label);
    setEvidence(place);
    setLocationError(undefined);
    setOpen(false);
    onChange(place);
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
          const place = unwrap(response, (body) => body.location);
          if (place) {
            choose(place);
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
      <div className="mb-1.5 flex items-center gap-2">
        <label htmlFor={`${listID}-input`} className="font-medium text-sm">
          Location
        </label>
        {locked && lockedLabel && (
          <span className="inline-flex items-center gap-1 rounded-sm bg-static-800 px-1.5 py-0.5 font-mono text-2xs text-static-400 uppercase tracking-wide">
            <Lock className="size-3" aria-hidden /> {lockedLabel}
          </span>
        )}
      </div>
      <div className="flex flex-col gap-2 sm:flex-row">
        <div className="relative min-w-0 flex-1">
          <Input
            id={`${listID}-input`}
            role="combobox"
            aria-autocomplete="list"
            aria-expanded={showResults}
            aria-controls={listID}
            aria-activedescendant={open && locations[active] ? `${listID}-${active}` : undefined}
            value={query}
            placeholder="Search for a city or country"
            autoComplete="off"
            disabled={locked}
            onFocus={() => setOpen(true)}
            onBlur={() => setOpen(false)}
            onChange={(event) => {
              setQuery(event.target.value);
              setEvidence(undefined);
              setOpen(true);
              setActive(0);
            }}
            onKeyDown={(event) => {
              if (!open || !choicesReady || locations.length === 0) return;
              if (event.key === "ArrowDown") {
                event.preventDefault();
                setActive((current) => Math.min(current + 1, locations.length - 1));
              } else if (event.key === "ArrowUp") {
                event.preventDefault();
                setActive((current) => Math.max(current - 1, 0));
              } else if (event.key === "Enter" && locations[active]) {
                event.preventDefault();
                choose(locations[active]);
              } else if (event.key === "Escape") {
                setOpen(false);
              }
            }}
          />
          {showResults && (
            <div
              id={listID}
              className="relative z-20 mt-1 w-full overflow-hidden rounded-md border border-border bg-popover text-popover-foreground shadow-lg sm:absolute"
            >
              {locations.length > 0 && (
                <div
                  role="listbox"
                  className={cn("max-h-64 overflow-auto p-1", !choicesReady && "opacity-50")}
                >
                  {locations.map((place, index) => (
                    <button
                      type="button"
                      id={`${listID}-${index}`}
                      key={place.id}
                      role="option"
                      aria-label={place.label}
                      tabIndex={-1}
                      aria-selected={index === active}
                      disabled={!choicesReady}
                      className={cn(
                        "flex w-full cursor-pointer items-center justify-between rounded-sm px-3 py-2 text-left text-sm disabled:cursor-default",
                        index === active ? "bg-accent text-accent-foreground" : "hover:bg-accent/70",
                      )}
                      onMouseDown={(event) => event.preventDefault()}
                      onClick={() => choose(place)}
                    >
                      <span className="flex min-w-0 flex-col">
                        <span className="truncate">{place.market || place.label}</span>
                        {place.market && (
                          <span className="truncate text-muted-foreground text-xs">
                            {[place.region, countryName(place.country)].filter(Boolean).join(", ")}
                          </span>
                        )}
                      </span>
                      {place.country === value.country && (place.market ?? "") === (value.market ?? "") && (
                        <Check className="ml-3 size-4 shrink-0" aria-hidden />
                      )}
                    </button>
                  ))}
                </div>
              )}
              {searching && (
                <div
                  className="flex items-center gap-2 border-border border-t px-3 py-2 text-muted-foreground text-xs"
                  role="status"
                >
                  <Loader2 className="size-3.5 animate-spin" aria-hidden />
                  Searching…
                </div>
              )}
              {!searching && locations.length === 0 && !search.isError && (
                <p className="px-3 py-3 text-muted-foreground text-sm">
                  No locations found. Try a nearby city or country.
                </p>
              )}
              {!searching && search.isError && (
                <p className="px-3 py-3 text-destructive text-sm" role="alert">
                  Location search is unavailable. Try again.
                </p>
              )}
            </div>
          )}
        </div>
        {allowDetection && (
          <Button
            type="button"
            variant="outline"
            onClick={() => void detect()}
            disabled={locked || detecting}
          >
            {detecting ? <Loader2 className="animate-spin" aria-hidden /> : <LocateFixed aria-hidden />}
            {detecting ? "Finding…" : "Use my location"}
          </Button>
        )}
      </div>
      {evidence?.approximate && (
        <p className="mt-2 text-muted-foreground text-sm">
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
        <p className="mt-2 text-destructive text-sm" role="alert">
          {visibleError}
        </p>
      )}
      {!value.country && !visibleError && <p className="mt-2 text-muted-foreground text-sm">{emptyHint}</p>}
    </div>
  );
};

export { LocationPicker };
