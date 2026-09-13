import { cn } from "@/lib/utils";
import type { InstallationLocationProps } from "./installation-location.type";
import { LocationPicker } from "./location-picker";

const COUNTRY_KEY = "filler.home_country";
const MARKET_KEY = "filler.home_market";

// Settings adapter around the shared location picker. Persisted setting keys, provenance, and
// save results stay here; place search and detection belong to LocationPicker.
const InstallationLocation = ({
  entries,
  values,
  onChange,
  results,
  error,
  card = false,
  showHeading = true,
}: InstallationLocationProps) => {
  const countryEntry = entries.find((entry) => entry.key === COUNTRY_KEY);
  const marketEntry = entries.find((entry) => entry.key === MARKET_KEY);
  const country = values[COUNTRY_KEY] ?? countryEntry?.value ?? "";
  const market = values[MARKET_KEY] ?? marketEntry?.value ?? "";
  const pinned = countryEntry?.provenance === "env" || marketEntry?.provenance === "env";
  const fieldResult = results?.find(
    (result) => (result.key === COUNTRY_KEY || result.key === MARKET_KEY) && result.status !== "saved",
  );

  return (
    <section
      className={cn(
        "flex flex-col gap-4",
        card && "rounded-xl border border-border bg-card p-5 shadow-sm sm:p-6",
      )}
    >
      {showHeading && (
        <div>
          <h2 className="font-semibold text-lg">Your location</h2>
          <p className="mt-1 max-w-xl text-muted-foreground text-sm">
            Where your channels are watched. Loomarr uses this to find filler that fits your area.
          </p>
        </div>
      )}

      <LocationPicker
        value={{ country, market }}
        onChange={(location) => {
          onChange(COUNTRY_KEY, location.country);
          onChange(MARKET_KEY, location.market ?? "");
        }}
        allowDetection
        locked={pinned}
        lockedLabel="set via environment"
        error={fieldResult?.problem ?? error}
      />
    </section>
  );
};

export { InstallationLocation };
