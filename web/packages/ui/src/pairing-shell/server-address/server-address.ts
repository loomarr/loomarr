/**
 * How a server's address reads on screen, discovered or entered: `host[:port]` without the scheme
 * or a trailing slash. The scheme is still what gets paired; this is only the label.
 */
const serverAddressLabel = (url: string) => url.replace(/^[a-z][a-z0-9+.-]*:\/\//i, "").replace(/\/+$/, "");

export { serverAddressLabel };
