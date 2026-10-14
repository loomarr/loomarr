import { createFileRoute } from "@tanstack/react-router";
import { PairDevice } from "@/settings/pair-device";

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

interface PairSearch {
  code?: string;
}

// A user code is a fixed shape (two groups of four from a restricted alphabet, dashes optional), so
// the boundary keeps only characters that shape allows and caps the length.
//
// This value is rendered into an input and posted to an endpoint that grants access, so it is
// sanitised HERE rather than trusted from the URL. The server normalises and validates it again —
// this is not the security boundary, it is the one that keeps junk out of the field.
const safeUserCode = (raw: unknown): string | undefined => {
  if (typeof raw !== "string") return undefined;
  const cleaned = raw
    .toUpperCase()
    .replace(/[^A-Z-]/g, "")
    .slice(0, 9);
  return cleaned || undefined;
};

const PairRoute = () => {
  const { code } = Route.useSearch();
  return <PairDevice initialCode={code} />;
};

const Route = createFileRoute("/pair")({
  validateSearch: (search: Record<string, unknown>): PairSearch => ({
    code: safeUserCode(search.code),
  }),
  component: PairRoute,
});

export { Route };
