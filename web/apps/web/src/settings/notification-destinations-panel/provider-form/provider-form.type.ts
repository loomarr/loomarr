import type { NotificationProviderDTO } from "@loomarr/api/models/notificationProviderDTO";
import type { NotificationProviderTypeDTO } from "@loomarr/api/models/notificationProviderTypeDTO";

/** What a provider form hands to be saved: the same shape whether it is created or updated. */
type ProviderSaveValue = {
  label: string;
  events: string[];
  enabled: boolean;
  settings: Record<string, string>;
};

type ProviderFormProps = {
  definition: NotificationProviderTypeDTO;
  provider?: NotificationProviderDTO;
  pending: boolean;
  error?: unknown;
  onCancel: () => void;
  onSave: (value: ProviderSaveValue) => Promise<void>;
};

export type { ProviderFormProps, ProviderSaveValue };
