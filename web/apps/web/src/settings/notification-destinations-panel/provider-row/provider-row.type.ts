import type { NotificationProviderDTO } from "@loomarr/api/models/notificationProviderDTO";
import type { NotificationProviderTypeDTO } from "@loomarr/api/models/notificationProviderTypeDTO";

type ProviderRowProps = {
  provider: NotificationProviderDTO;
  definition?: NotificationProviderTypeDTO;
};

export type { ProviderRowProps };
