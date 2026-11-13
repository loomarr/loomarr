import { Children, type ReactNode } from "react";

// Whether a cell holds only text, so it takes the cell's voice; anything else (a button, a status
// word) brings its own. Several strings count: a Markdown renderer hands a cell its text in an array.
const isPlainText = (children: ReactNode) => {
  const parts = Children.toArray(children);
  return parts.length > 0 && parts.every((part) => typeof part === "string" || typeof part === "number");
};

export { isPlainText };
