import path from "node:path";

/**
 * Rewrite relative links for the published docs site.
 *
 * Pages link each other as relative `.md` files ("../guides/filler.md#pipeline"), the one form
 * that works on GitHub, in the in-app help and here. Starlight serves each file as a route
 * directory, so the same relative path would point below the route. This adapter resolves the
 * link against the source file: a published page becomes its site route, and anything else in
 * the repository (an unpublished doc, a compose file) becomes its GitHub URL.
 */
export default function remarkMdLinks({ base, docsRoot, include, repoUrl }) {
  const siteBase = base.endsWith("/") ? base.slice(0, -1) : base;
  const repoRoot = path.dirname(docsRoot);
  const published = (rel) =>
    include.some((entry) => rel === entry || rel.startsWith(`${entry}/`));

  return (tree, file) => {
    const source = file?.path ?? file?.history?.[0];
    if (!source) return;
    walk(tree, (node) => {
      if (node.type !== "link" || !isRelative(node.url)) return;
      const [target, fragment] = node.url.split("#", 2);
      const hash = fragment ? `#${fragment}` : "";
      const abs = path.resolve(path.dirname(source), target);
      const rel = path.relative(docsRoot, abs).split(path.sep).join("/");
      if (target.endsWith(".md") && !rel.startsWith("..") && published(rel)) {
        const route = rel.replace(/\.md$/, "").replace(/(^|\/)index$/, "");
        node.url = `${siteBase}/${route ? `${route}/` : ""}${hash}`;
        return;
      }
      if (/\.(d2|svg)$/.test(target) && rel.startsWith("diagrams/")) return;
      const repoPath = path.relative(repoRoot, abs).split(path.sep).join("/");
      node.url = `${repoUrl}/blob/main/${repoPath}${hash}`;
    });
  };
}

function isRelative(url) {
  return (
    typeof url === "string" &&
    url !== "" &&
    !/^[a-z][a-z0-9+.-]*:/i.test(url) &&
    !url.startsWith("/") &&
    !url.startsWith("#")
  );
}

function walk(node, visit) {
  visit(node);
  if (!Array.isArray(node.children)) return;
  for (const child of node.children) walk(child, visit);
}
