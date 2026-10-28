import type { LocationDTO } from "@loomarr/api/models/locationDTO";

interface LocationValue {
  country: string;
  place?: string;
}

interface LocationPickerProps {
  value: LocationValue;
  onChange: (location: LocationDTO) => void;
  allowDetection?: boolean;
  emptyHint?: string;
  locked?: boolean;
  lockedLabel?: string;
  error?: string;
  // Only results in this country are offered (the installation country, for a channel).
  restrictCountry?: string;
}

export type { LocationPickerProps, LocationValue };
