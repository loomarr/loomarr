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

export { anchorOf };
