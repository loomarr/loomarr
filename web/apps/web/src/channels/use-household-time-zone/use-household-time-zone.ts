import * as channelsApi from "@loomarr/api/endpoints/channels";
import { unwrap } from "@loomarr/api/unwrap";
import { useGuideWindow } from "@loomarr/core/guide";
import { keepPreviousData } from "@tanstack/react-query";

/**
 * The household's timezone, which the Guide tells times in, so a time shown elsewhere reads the same
 * as the Guide's for a viewer in another zone. It comes from the same pinned-window guide read that
 * the Guide, Home and Watch share, so it is usually already cached. Undefined until that read
 * answers; a time formatter falls back to the device's zone in the meantime.
 */
const useHouseholdTimeZone = (): string | undefined => {
  const { window } = useGuideWindow();
  const guide = channelsApi.useChannelGuide(window, {
    query: { retry: false, placeholderData: keepPreviousData },
  });
  return unwrap(guide.data)?.timezone;
};

export { useHouseholdTimeZone };
