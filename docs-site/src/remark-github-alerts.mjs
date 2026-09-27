// PROTOTYPE: render GitHub alert blockquotes (> [!NOTE], > [!TIP], > [!WARNING], > [!CAUTION])
// as Starlight asides. One Markdown syntax then reads right on GitHub, on the site and in the
// in-app help (which can fall back to a plain blockquote).
const KINDS = { NOTE: "note", TIP: "tip", IMPORTANT: "note", WARNING: "caution", CAUTION: "danger" };
const TITLES = { note: "Note", tip: "Tip", caution: "Caution", danger: "Danger" };

const walk = (node, visit) => {
  if (!node.children) return;
  node.children.forEach((child, i) => {
    visit(child, i, node);
    walk(child, visit);
  });
};

export default function remarkGithubAlerts() {
  return (tree) => {
    walk(tree, (node, index, parent) => {
      if (node.type !== "blockquote") return;
      const first = node.children[0];
      const text = first?.type === "paragraph" ? first.children[0] : null;
      const match = text?.type === "text" && text.value.match(/^\[!(\w+)\]\s*\n?/);
      if (!match || !KINDS[match[1]]) return;
      const kind = KINDS[match[1]];
      text.value = text.value.slice(match[0].length);
      parent.children[index] = {
        type: "aside",
        data: {
          hName: "aside",
          hProperties: { "aria-label": TITLES[kind], className: ["starlight-aside", `starlight-aside--${kind}`] },
        },
        children: [
          {
            type: "paragraph",
            data: { hName: "p", hProperties: { className: ["starlight-aside__title"], ariaHidden: "true" } },
            children: [{ type: "text", value: TITLES[kind] }],
          },
          { type: "aside-content", data: { hName: "div", hProperties: { className: ["starlight-aside__content"] } }, children: node.children },
        ],
      };
    });
  };
}
