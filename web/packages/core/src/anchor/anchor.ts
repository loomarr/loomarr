// anchorOf converts a markdown heading to its fragment id — lowercased, with runs of
// non-alphanumerics collapsed to a single hyphen.
//
// THIS IS A CROSS-LANGUAGE CONTRACT. The Go side computes the same slug (docs/embed.go
// `Anchor`) to validate that every `docHref` the API emits — "troubleshooting#tunarr" on
// a failed setup check — resolves to a real heading. §13 promises "every red check
// deep-links to its section", and that promise holds only if the ids RENDERED here match
// the anchors VALIDATED there. Keep the two implementations identical; anchor.test.ts
// checks this one against the same document the Go test checks.
const anchorOf = (heading: string): string => {
  const out: string[] = [];
  let lastHyphen = false;
  for (const ch of heading.toLowerCase()) {
    if ((ch >= "a" && ch <= "z") || (ch >= "0" && ch <= "9")) {
      out.push(ch);
      lastHyphen = false;
    } else if (ch === " " || ch === "-" || ch === "_") {
      if (!lastHyphen && out.length > 0) {
        out.push("-");
        lastHyphen = true;
      }
    }
  }
  return out.join("").replace(/-$/, "");
};

// parseDocHref splits a help deep-link — the shape the API emits on a failed setup check
// ("troubleshooting#tunarr"), and the shape in-doc cross-links use ("concepts",
// "member-guide#reading-channel-status") — into the help route's search params.
//
// It exists because the raw string cannot be used as an href: "troubleshooting#tunarr" as
// an <a href> resolves RELATIVE to the current path (from /settings it becomes
// /settings/troubleshooting), and passing the whole string as ?page= makes it a bogus
// slug. §13 promises "every red check deep-links to its section"; that only holds if the
// link is turned into { page, section } and routed. A leading "#" (a same-page anchor)
// yields no page and is left to the browser.
const parseDocHref = (href: string): { page?: string; section?: string } => {
  if (href.startsWith("#")) return { section: href.slice(1) || undefined };
  const [page, section] = href.split("#", 2);
  return { page: page || undefined, section: section || undefined };
};

export { anchorOf, parseDocHref };
