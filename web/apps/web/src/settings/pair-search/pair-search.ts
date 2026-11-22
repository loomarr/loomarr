// The `/pair` page's query string, validated at the route boundary.

interface PairSearch {
  code?: string;
  next?: string;
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

// `next` is where an approval continues when an app's OAuth sign-in started it (#4485): the
// server's own continue step, which hands the app its one-time code. The page follows it with a
// full navigation right after someone grants access, so ONLY that one same-origin path, with an
// opaque request id, is accepted. Anything else would make `/pair` an open redirect at the moment
// it is most trusted.
const continuePath = /^\/v1\/oauth\/continue\?request=[A-Za-z0-9_-]{16,128}$/;

const safeNext = (raw: unknown): string | undefined =>
  typeof raw === "string" && continuePath.test(raw) ? raw : undefined;

const validatePairSearch = (search: Record<string, unknown>): PairSearch => ({
  code: safeUserCode(search.code),
  next: safeNext(search.next),
});

export type { PairSearch };
export { validatePairSearch };
