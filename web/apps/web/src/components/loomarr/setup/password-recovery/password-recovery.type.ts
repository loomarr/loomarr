interface PasswordRecoveryRequestProps {
  isPending?: boolean;
  sent?: boolean;
  error?: unknown;
  onSubmit: (username: string) => void;
}

interface PasswordRecoveryResetProps {
  expiresAt?: number;
  isLoading?: boolean;
  isRedeeming?: boolean;
  succeeded?: boolean;
  error?: unknown;
  onRedeem: (password: string) => void;
}

export type { PasswordRecoveryRequestProps, PasswordRecoveryResetProps };
