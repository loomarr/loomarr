import type { LoginInputBody } from "@loomarr/api";

interface LoginFormProps {
  // The generated request DTO (§12, 1:1 contract) — never a hand-mirrored shape.
  onSubmit: (values: LoginInputBody) => void;
  isPending?: boolean;
  // The block-level failure (RFC 7807 ApiError, or any Error) rendered via ErrorState.
  error?: unknown;
  className?: string;
}

export type { LoginFormProps };
