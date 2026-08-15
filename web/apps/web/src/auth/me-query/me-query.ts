import * as authApi from "@loomarr/api/endpoints/auth";

// The single definition of the GET /v1/auth/me query, shared by useAuth (render-time
// identity) and the router beforeLoad guards (ensureQueryData in _authed/login).
// retry:false — a 401 is a definite answer (signed out), not a transient failure (§11).
const meQueryOptions = () => authApi.getMeQueryOptions({ query: { retry: false } });

export { meQueryOptions };
