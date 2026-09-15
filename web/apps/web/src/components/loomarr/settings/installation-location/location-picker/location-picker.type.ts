import type { LocationDTO } from "@loomarr/api/models/locationDTO";

interface LocationValue {
  country: string;
  market?: string;
}

interface LocationPickerProps {
  value: LocationValue;
  onChange: (location: LocationDTO) => void;
  allowDetection?: boolean;
  locked?: boolean;
  lockedLabel?: string;
  error?: string;
}

export type { LocationPickerProps, LocationValue };
