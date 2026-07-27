import type { LoginInputBody } from "@loomarr/api";

interface LoginFormProps {
  // The generated request DTO (§12, 1:1 contract) — never a hand-mirrored shape.
  onSubmit: (values: LoginInputBody) => void;
  isPending?: boolean;
  // The block-level failure (RFC 7807 ApiError, or any Error) rendered via ErrorState.
  error?: unknown;
  // Offers the credential-free dev sign-in (§11). Supplied by the route from
  // `GET /v1/setup/state`'s `devLogin`, which mirrors what the server actually mounted
  // — so the affordance can never appear pointing at a route that 404s. Undefined
  // (the default) renders nothing, which is every shipped install.
  onDevLogin?: () => void;
  className?: string;
}

export type { LoginFormProps };
