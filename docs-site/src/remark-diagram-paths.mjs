/**
 * Rewrite repository-relative diagram links for the published docs site.
 *
 * GitHub resolves paths from the Markdown file. Starlight maps those same files to route
 * directories, so their relative paths would point below the route instead. Astro serves the
 * original docs/diagrams directory as its public tree; this adapter changes only the site AST.
 */
export default function remarkDiagramPaths({ base }) {
  const siteBase = base.endsWith("/") ? base.slice(0, -1) : base;

  return (tree) => {
    walk(tree, (node) => {
      if (node.type === "image" && isGeneratedDiagram(node.url)) {
        node.url = `${siteBase}/generated/${fileName(node.url)}`;
      }
      if (node.type === "link" && isDiagramSource(node.url)) {
        node.url = `${siteBase}/${fileName(node.url)}`;
      }
    });
  };
}

function walk(node, visit) {
  visit(node);
  if (!Array.isArray(node.children)) return;
  for (const child of node.children) walk(child, visit);
}

function isGeneratedDiagram(url) {
  return typeof url === "string" && url.includes("diagrams/generated/") && url.endsWith(".svg");
}

function isDiagramSource(url) {
  return typeof url === "string" && url.includes("diagrams/") && url.endsWith(".d2");
}

function fileName(url) {
  return url.slice(url.lastIndexOf("/") + 1);
}
