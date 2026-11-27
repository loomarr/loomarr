import type { NotificationProviderFieldDTO } from "@loomarr/api/models/notificationProviderFieldDTO";

type ProviderFieldInputProps = {
  field: NotificationProviderFieldDTO;
  inputId: string;
  value: string;
  configured: boolean;
  cleared: boolean;
  error?: string;
  onChange: (value: string) => void;
  onClear: () => void;
};

export type { ProviderFieldInputProps };
