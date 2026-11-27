import { createFileRoute } from "@tanstack/react-router";
import { PairDevice } from "@/settings/pair-device";
import { validatePairSearch } from "@/settings/pair-search";

// `/pair` — the address a television displays (§11, Shield P1).
//
// ⚠ TOP-LEVEL, deliberately NOT under `_authed`, which would wrap it in AppShell. This is a
// single-task landing page: someone is standing with a remote in one hand and a phone in the
// other, and the only thing they came to do is type eight characters. Dashboard/Guide/Queue/
// Filler/People/Settings/Help around that is noise, and for a MEMBER it is worse than noise —
// several of those destinations are admin-gated, so the chrome would offer doors that do not open.
//
// `/login` and `/wizard` sit out here for exactly the same reason. A focused task page is not an
// application surface, and the shell is what separates the two.
//
// Signed-out visitors are handled by the component (it prompts sign-in and returns here with the
// code intact) rather than by a route guard, because being signed out is the EXPECTED arrival
// state: the person holding the remote is often not yet signed in on the phone they are typing on.
//
// An app's OAuth sign-in (#4485) also lands here, with `next` naming the server step that finishes
// it; see validatePairSearch for why only that one path is accepted.

const PairRoute = () => {
  const { code, next } = Route.useSearch();
  return <PairDevice initialCode={code} next={next} />;
};

// no-loader: approving a code is a mutation; the page reads nothing
const Route = createFileRoute("/pair")({
  validateSearch: validatePairSearch,
  component: PairRoute,
});

export { Route };
