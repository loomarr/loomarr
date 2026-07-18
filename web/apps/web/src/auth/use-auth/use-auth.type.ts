import type { MeBody } from "@loomarr/api";

// The interpreted session identity every guard + role-gated surface reads (§11).
interface AuthState {
  user: MeBody | undefined;
  isAuthenticated: boolean;
  isAdmin: boolean;
  isLoading: boolean;
  error: unknown;
}

export type { AuthState };
