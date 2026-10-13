// A responder answers with whatever public URL it was configured with, so a backend left on
// `http://localhost:<port>` advertises its own loopback. The device can't reach another machine's
// loopback, nor a link-local address without its interface zone, so neither is offered.
const isReachableServerUrl = (url: string): boolean => {
  // Parsed by hand: React Native's URL polyfill does not implement `hostname`.
  const host = /^https?:\/\/(?:[^@/?#]*@)?(\[[^\]]*\]|[^/:?#]+)/i.exec(url)?.[1]?.toLowerCase();
  if (!host) return false;
  if (host === "localhost" || host.endsWith(".localhost")) return false;
  if (host.startsWith("[")) {
    const address = host.slice(1, -1).split("%")[0] ?? "";
    if (address === "::" || address === "::1" || /^fe[89ab][0-9a-f]:/.test(address)) return false;
    const mapped = /^::ffff:(\d+\.\d+\.\d+\.\d+)$/.exec(address)?.[1];
    return mapped === undefined || isReachableIPv4(mapped);
  }
  return isReachableIPv4(host);
};

const isReachableIPv4 = (host: string) => {
  const octets = host.split(".");
  if (octets.length !== 4 || !octets.every((octet) => /^\d{1,3}$/.test(octet))) return true;
  const [first, second] = octets.map(Number);
  return !(first === 0 || first === 127 || (first === 169 && second === 254));
};

export { isReachableServerUrl };
