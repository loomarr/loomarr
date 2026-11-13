import * as notificationsApi from "@loomarr/api/endpoints/notifications";
import { useQueryClient } from "@tanstack/react-query";

/** Reads the provider list again after a change, so the panel shows what the server now holds. */
const useRefreshProviders = () => {
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: notificationsApi.getNotificationProvidersListQueryKey() });
};

export { useRefreshProviders };
