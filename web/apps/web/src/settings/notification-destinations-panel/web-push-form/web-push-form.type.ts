import type { NotificationProviderDTO } from "@loomarr/api/models/notificationProviderDTO";
import type { NotificationProviderTypeDTO } from "@loomarr/api/models/notificationProviderTypeDTO";
import type { ProviderSaveValue } from "../provider-form";

type WebPushFormProps = {
  definition: NotificationProviderTypeDTO;
  provider?: NotificationProviderDTO;
  publicKey?: string;
  pending: boolean;
  error?: unknown;
  onCancel: () => void;
  onSave: (value: ProviderSaveValue) => Promise<void>;
};

export type { WebPushFormProps };
