import { createFileRoute, redirect } from "@tanstack/react-router";
import { parseOriginationSearch } from "@/suggest/origination-flow";
import { RequestNewPage } from "@/suggest/request-new-page";

// Request a channel (§12): the describe → review → approve flow, on its own path under Requests so
// the nav keeps Requests lit and the URL says what it is (it lived on `/guide?new=%221%22`). The
// search params are the flow's state (OriginationSearch): `?job=` follows a request, `?manual=1` is
// the builder, `?intent=` seeds the form (the wizard's guided first channel hands off with it).
const RequestNewScreen = () => <RequestNewPage search={Route.useSearch()} />;

const Route = createFileRoute("/_authed/requests/new")({
  component: RequestNewScreen,
  validateSearch: parseOriginationSearch,
  // Arriving with no params resumes the Job this tab was following. Only on ENTRY: a flow step
  // navigates within the route and must not bounce back to the remembered Job.
  // The remembered run is imported on entry only, so the draft's zod schema stays out of the first
  // download.
  beforeLoad: async ({ search, cause }) => {
    if (cause !== "enter" || Object.values(search).some((value) => value !== undefined)) return;
    const { rememberedOrigination } = await import("@/suggest/remembered-origination");
    const remembered = rememberedOrigination();
    if (remembered) throw redirect({ to: "/requests/new", search: remembered, replace: true });
  },
});

export { Route };
