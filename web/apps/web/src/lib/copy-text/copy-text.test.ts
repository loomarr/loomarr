import { afterEach, describe, expect, it, vi } from "vitest";
import { copyText } from "./copy-text";

const setClipboard = (value: unknown) =>
  Object.defineProperty(navigator, "clipboard", { configurable: true, value });
const setExec = (impl: (command: string) => boolean) =>
  Object.defineProperty(document, "execCommand", { configurable: true, value: vi.fn(impl) });

afterEach(() => {
  setClipboard(undefined);
  Reflect.deleteProperty(document, "execCommand");
});

describe("copyText", () => {
  it("writes through the clipboard API where there is one", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    setClipboard({ writeText });

    expect(await copyText("abc")).toBe(true);
    expect(writeText).toHaveBeenCalledWith("abc");
  });

  it("falls back to a selection copy on an insecure origin, with no clipboard API", async () => {
    setClipboard(undefined);
    let copiedText: string | undefined;
    setExec(() => {
      copiedText = (document.querySelector("textarea") as HTMLTextAreaElement).value;
      return true;
    });

    expect(await copyText("lan-text")).toBe(true);
    expect(copiedText).toBe("lan-text");
    expect(document.querySelector("textarea")).toBeNull();
  });

  it("falls back when the clipboard API refuses", async () => {
    setClipboard({ writeText: vi.fn().mockRejectedValue(new Error("denied")) });
    setExec(() => true);

    expect(await copyText("x")).toBe(true);
  });

  it("reports failure when nothing could write, and leaves no field behind", async () => {
    setClipboard(undefined);
    setExec(() => false);

    expect(await copyText("x")).toBe(false);
    expect(document.querySelector("textarea")).toBeNull();
  });
});
