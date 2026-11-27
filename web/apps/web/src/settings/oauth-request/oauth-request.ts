import { customFetch } from "@loomarr/api/mutator";
import { useQuery } from "@tanstack/react-query";

// The app behind an OAuth sign-in (#4485), read BEFORE anyone approves so /pair can say who is
// asking and where the grant goes. Without it the screen could only say "an app", and a link from
// anyone would look the same as the app the person just connected.
//
// The route is hidden from the OpenAPI document like its OAuth siblings (their contract is an RFC,
// not ours), so there is no generated hook; customFetch is the same transport the generated hooks
// use, with the same credentials and typed ApiError.

interface OAuthRequest {
  // The client's self-chosen name, cleaned by the server. A claim, which is why sendsTo is shown.
  app: string;
  // The host the one-time code is sent to: the fact the person actually checks.
  sendsTo: string;
  // The code goes to an app on the computer this browser runs on, such as a CLI.
  loopback: boolean;
}

const requestIdOf = (next: string): string =>
  new URLSearchParams(next.split("?")[1] ?? "").get("request") ?? "";

// `next` is the validated continue path from the /pair URL; pass undefined to skip the read (no
// sign-in waiting, or not signed in yet, when the route would answer 401).
const useOAuthRequest = (next: string | undefined) =>
  useQuery({
    queryKey: ["oauth-request", next],
    enabled: Boolean(next),
    retry: false,
    queryFn: async () => {
      const id = encodeURIComponent(requestIdOf(next ?? ""));
      const res = await customFetch<{ data: OAuthRequest }>(`/v1/oauth/request?request=${id}`);
      return res.data;
    },
  });

export type { OAuthRequest };
export { useOAuthRequest };
