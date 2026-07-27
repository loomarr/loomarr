import { createFileRoute } from "@tanstack/react-router";
import { GuidePage } from "@/channels";

// The channels surface (§12) — headed "Channels", it is both the cross-channel time grid and
// the app's one origination door. Readable by any authenticated user: the guide is
// viewer-facing, and GET /v1/guide is likewise not admin-gated.
//
// `?intent=` is part of the route contract. The wizard's guided first channel hands off here
// with a template prefilled (§13's blank-page killer) — the page forwards it into the inline
// describe panel and opens it, so the handoff lands on a filled form rather than an empty grid
// with the operator wondering where their template went.
interface GuideSearch {
  intent?: string;
}

const GuideScreen = () => {
  const { intent } = Route.useSearch();
  return <GuidePage initialIntent={intent} />;
};

const Route = createFileRoute("/_authed/guide")({
  component: GuideScreen,
  validateSearch: (search: Record<string, unknown>): GuideSearch => ({
    intent: typeof search.intent === "string" ? search.intent : undefined,
  }),
});

export { Route };
