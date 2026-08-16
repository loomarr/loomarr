import type { Intent } from "@loomarr/api/models/intent";

type GuidePageProps = {
  // Seeds the inline describe panel and opens it on arrival. The route resolves either a stable
  // preset id or a legacy `?intent=` link into this one typed value.
  initialIntent?: Intent;
};

export type { GuidePageProps };
