// What the site publishes from docs/, one list for the content loader and the link adapter.
//
// ⚠ Explicit, not a glob: docs/agents/ (agent contracts, hash-pinned by the vendored skills)
// and project/ (plans and evidence) must stay off the site, and a new folder should be a
// decision rather than an accident. design.md and design/ belong to the design lane (#779);
// they are published under Contributing → Design.
export const PUBLISHED = [
  "get-started.md",
  "guides",
  "reference",
  "explanation",
  "contributing",
  "design.md",
  "design",
  "programming-design.md",
  "config-design.md",
  "frontend-design.md",
];
