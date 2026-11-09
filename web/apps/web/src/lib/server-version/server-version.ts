// The sidebar's version line (#2751): the release tag, or "dev · <short commit>" for a build that
// is not a release (the API reports "dev" for an unstamped build). Never inferred as "staging".
const serverVersionLabel = (body: { version: string; commit?: string; dirty?: boolean }): string => {
  const base =
    body.version === "dev" || body.version === ""
      ? `dev${body.commit ? ` · ${body.commit.slice(0, 7)}` : ""}`
      : body.version;
  return `${base}${body.dirty ? " (modified)" : ""}`;
};

export { serverVersionLabel };
