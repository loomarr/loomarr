import { SettingKey } from "@loomarr/api/models/settingKey";
import { Stack, Text } from "@loomarr/design-system";
import { SettingField } from "@/components/loomarr/settings/setting-field";
import { cn } from "@/lib/utils";
import type { InstallationLocationProps } from "./installation-location.type";
import { LanguagePicker } from "./language-picker";
import { LocationPicker } from "./location-picker";

const COUNTRY_KEY = SettingKey.FillerHomeCountry;
const PLACE_KEY = SettingKey.FillerHomePlace;
const RADIUS_KEY = SettingKey.FillerLocalRadiusKm;
const LANGUAGE_KEY = SettingKey.FillerLanguage;

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
  const placeEntry = entries.find((entry) => entry.key === PLACE_KEY);
  const radiusEntry = entries.find((entry) => entry.key === RADIUS_KEY);
  const languageEntry = entries.find((entry) => entry.key === LANGUAGE_KEY);
  const country = values[COUNTRY_KEY] ?? countryEntry?.value ?? "";
  const place = values[PLACE_KEY] ?? placeEntry?.value ?? "";
  const radius = values[RADIUS_KEY] ?? radiusEntry?.value ?? "";
  const language = values[LANGUAGE_KEY] ?? languageEntry?.value ?? "en";
  const pinned = countryEntry?.provenance === "env" || placeEntry?.provenance === "env";
  const fieldResult = results?.find(
    (result) => (result.key === COUNTRY_KEY || result.key === PLACE_KEY) && result.status !== "saved",
  );
  const radiusResult = results?.find((result) => result.key === RADIUS_KEY && result.status !== "saved");

  return (
    <Stack
      className={cn(card && "border-border bg-card rounded-xl border p-5 shadow-sm sm:p-6")}
      gap="4"
      render="section"
    >
      {showHeading && (
        <div>
          <Text as="h2" strong textRole="headline">
            Your area and language
          </Text>
          <Text as="p" className="mt-1 max-w-xl" textRole="compact" tone="secondary">
            Loomarr uses these to find commercials that fit your home.
          </Text>
        </div>
      )}

      <LocationPicker
        value={{ country, place }}
        onChange={(location) => {
          onChange(COUNTRY_KEY, location.country);
          onChange(PLACE_KEY, location.place ?? "");
        }}
        allowDetection
        locked={pinned}
        lockedLabel="set via environment"
        error={fieldResult?.problem ?? error}
      />

      {radiusEntry && (
        <SettingField
          entry={radiusEntry}
          value={radius}
          onChange={(value) => onChange(RADIUS_KEY, value)}
          result={radiusResult}
        />
      )}

      {languageEntry && (
        <LanguagePicker
          value={language}
          options={languageEntry.enum ?? []}
          onChange={(next) => onChange(LANGUAGE_KEY, next)}
          locked={languageEntry.provenance === "env"}
          lockedLabel="set via environment"
        />
      )}
    </Stack>
  );
};

export { InstallationLocation };
