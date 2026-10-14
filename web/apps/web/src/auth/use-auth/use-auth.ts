import { useQuery } from "@tanstack/react-query";
import { meQueryOptions } from "../me-query";
import type { AuthState } from "./use-auth.type";

// useAuth — the one interpreter of GET /v1/auth/me (§11). AppShell, the Layout, and
// every role-gated surface read identity from here (the router beforeLoad guards read
// the same query via meQueryOptions). meResponse is a success|error union (customFetch
// throws on non-2xx, so a resolved value is always the 200 variant — narrow by status
// to reach MeBody, not ErrorModel).
const useAuth = (): AuthState => {
  const query = useQuery(meQueryOptions());
  const res = query.data;
  const user = res?.status === 200 ? res.data : undefined;
  return {
    user,
    isAuthenticated: user !== undefined,
    isAdmin: user?.role === "admin",
    isLoading: query.isLoading,
    error: query.error,
  };
};

export { useAuth };
