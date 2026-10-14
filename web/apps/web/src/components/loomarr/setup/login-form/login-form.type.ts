import type { LoginInputBody } from "@loomarr/api/models/loginInputBody";

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
  // Offers single sign-on (§11, V8). Supplied by the route from `GET /v1/setup/state`'s
  // `sso`, so — like onDevLogin — the button cannot appear pointing at a route that is not
  // mounted. Undefined renders nothing, which is every install with no provider configured.
  onSso?: () => void;
  // A reason code from a refused SSO round trip (`?sso=` on the login URL), turned into copy
  // HERE rather than sent as a message: reflecting server text into a page the browser
  // renders is how a redirect becomes a phishing surface.
  ssoError?: string;
  className?: string;
}

export type { LoginFormProps };
