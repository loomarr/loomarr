// Crash reports leave the device, so everything in one is scrubbed first: a URL keeps its path only
// (signed HLS play URLs carry their token in the query), and credential-shaped text is redacted.
// The server scrubs again; this side keeps the secret off the wire in the first place.

const ABSOLUTE_URL = /\b[a-z][a-z0-9+.-]*:\/\/[^\s"'`<>()[\]{}]*/gi;
const PATH_WITH_QUERY = /(\/[^\s?#"'`<>()[\]{}]*)[?#][^\s"'`<>()[\]{}]*/g;
// A stack frame's URL ends in :line:column after its query (`index.bundle?platform=ios:12:34`).
const FRAME_POSITION = /(?::\d+){2}$/;
const SECRET_HEADER = /\b(authorization|proxy-authorization|cookie|set-cookie|x-api-key)(\s*[:=]\s*)[^\n]*/gi;
const BEARER = /\bbearer\s+[\w.~+/=-]+/gi;
const SECRET_PARAMETER =
  /\b(token|access_token|refresh_token|api_key|apikey|sig|signature|password)=[^&\s"']+/gi;
const REDACTED = "[redacted]";

/** The path of a URL, without scheme, host, query or fragment. A bare path keeps its path. */
const scrubUrl = (url: string): string => {
  const withoutAuthority = url.replace(/^[a-z][a-z0-9+.-]*:\/\/[^/?#]*/i, "");
  const path = withoutAuthority.split(/[?#]/, 1)[0] ?? "";
  return path === "" ? "/" : path;
};

const scrubMatchedUrl = (match: string) => scrubUrl(match) + (FRAME_POSITION.exec(match)?.[0] ?? "");

/** Free text (an error message, a stack) with every URL reduced to its path and secrets redacted. */
const scrubText = (text: string): string =>
  text
    .replace(SECRET_HEADER, (_match, name: string, separator: string) => `${name}${separator}${REDACTED}`)
    .replace(BEARER, `Bearer ${REDACTED}`)
    .replace(ABSOLUTE_URL, scrubMatchedUrl)
    .replace(PATH_WITH_QUERY, (match, path: string) => path + (FRAME_POSITION.exec(match)?.[0] ?? ""))
    .replace(SECRET_PARAMETER, (_match, name: string) => `${name}=${REDACTED}`);

const utf8Length = (codePoint: number) =>
  codePoint < 0x80 ? 1 : codePoint < 0x800 ? 2 : codePoint < 0x10000 ? 3 : 4;

/** The longest prefix of `text` that fits `maxBytes` of UTF-8, never splitting a character. */
const truncateUtf8 = (text: string, maxBytes: number): string => {
  let bytes = 0;
  let end = 0;
  for (const character of text) {
    bytes += utf8Length(character.codePointAt(0) ?? 0);
    if (bytes > maxBytes) return text.slice(0, end);
    end += character.length;
  }
  return text;
};

const utf8ByteLength = (text: string): number => {
  let bytes = 0;
  for (const character of text) bytes += utf8Length(character.codePointAt(0) ?? 0);
  return bytes;
};

export { scrubText, scrubUrl, truncateUtf8, utf8ByteLength };
