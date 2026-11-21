// The one way the web puts text on the clipboard. `navigator.clipboard` exists only on a secure
// origin (https or localhost), and a plain-HTTP LAN install is a supported deployment, so there
// the old selection-and-copy command is the fallback. The answer is honest: true only when the
// text was written, so a caller never says "Copied" over nothing.

// A hidden textarea, selected and copied by the browser's own command, then removed.
const copyBySelection = (text: string): boolean => {
  const field = document.createElement("textarea");
  field.value = text;
  field.setAttribute("readonly", "");
  field.setAttribute("aria-hidden", "true");
  field.style.position = "fixed";
  field.style.top = "0";
  field.style.left = "-9999px";
  field.style.opacity = "0";
  const focused = document.activeElement instanceof HTMLElement ? document.activeElement : undefined;
  document.body.appendChild(field);
  try {
    field.focus();
    field.select();
    return typeof document.execCommand === "function" && document.execCommand("copy");
  } catch {
    return false;
  } finally {
    field.remove();
    focused?.focus();
  }
};

const copyText = async (text: string): Promise<boolean> => {
  if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Refused (permission, or a page without focus): the selection route may still work.
    }
  }
  return copyBySelection(text);
};

export { copyText };
