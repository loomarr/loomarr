import * as channelsApi from "@loomarr/api/endpoints/channels";
import { unwrap } from "@loomarr/api/unwrap";

/**
 * The household's timezone, which the Guide tells times in, so a time shown elsewhere reads the same
 * as the Guide's for a viewer in another zone. It is its own small read (`GET /v1/household`), kept
 * for the session: a setting an admin rarely changes is not worth a Guide load or a refetch per
 * event. Undefined until that read answers, or when no timezone is set; a time formatter then falls
 * back to the device's zone.
 */
const useHouseholdTimeZone = (): string | undefined => {
  const household = channelsApi.useHousehold({
    query: { retry: false, staleTime: Number.POSITIVE_INFINITY },
  });
  return unwrap(household.data)?.timeZone || undefined;
};

export { useHouseholdTimeZone };
