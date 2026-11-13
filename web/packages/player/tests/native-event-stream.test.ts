import type { NativeEventRequest } from "@loomarr/player/native";
import { describe, expect, it, vi } from "vitest";

vi.mock("expo-video", () => ({
  createVideoPlayer: vi.fn(),
  VideoView: vi.fn(),
}));

const { createNativeEventStream, parseEventFrames } = await import("@loomarr/player/native");

class RequestStub implements NativeEventRequest {
  aborted = false;
  headers = new Map<string, string>();
  method = "";
  onerror: ((event: ProgressEvent<EventTarget>) => unknown) | null = null;
  onreadystatechange: ((event: Event) => unknown) | null = null;
  readyState = 0;
  responseText = "";
  status = 0;
  url = "";

  abort = () => {
    this.aborted = true;
  };
  open = (method: string, url: string) => {
    this.method = method;
    this.url = url;
  };
  send = vi.fn();
  setRequestHeader = (name: string, value: string) => {
    this.headers.set(name, value);
  };
}

const timerHarness = () => {
  const callbacks: Array<() => void> = [];
  return {
    callbacks,
    clearTimer: vi.fn(),
    setTimer: vi.fn((callback: () => void) => {
      callbacks.push(callback);
      return callbacks.length as unknown as ReturnType<typeof setTimeout>;
    }),
  };
};

describe("native event stream", () => {
  it("parses named, multiline, comment, empty, CRLF, and incomplete SSE frames", () => {
    expect(
      parseEventFrames(
        'event: channel\r\ndata: {"id":\r\ndata: "seven"}\r\n\r\n: keepalive\r\n\r\nevent:\ndata:\n\npartial',
      ),
    ).toEqual({
      frames: [
        { data: '{"id":\n"seven"}', type: "channel" },
        { data: "", type: "message" },
      ],
      rest: "partial",
    });
  });

  it("authenticates, enforces SSE headers, dispatches incremental frames, and closes cleanly", () => {
    const timers = timerHarness();
    const requests: RequestStub[] = [];
    const stream = createNativeEventStream("http://loomarr.test/v1/events", {
      clearTimer: timers.clearTimer,
      createRequest: () => {
        const request = new RequestStub();
        requests.push(request);
        return request;
      },
      headers: { Accept: "text/plain", Authorization: "Bearer device-token" },
      setTimer: timers.setTimer,
    });
    const frames = vi.fn();
    stream.addEventListener("channel", frames);

    expect(timers.setTimer).toHaveBeenCalledWith(expect.any(Function), 0);
    timers.callbacks.shift()?.();
    const request = requests[0]!;
    expect(request).toMatchObject({ method: "GET", url: "http://loomarr.test/v1/events" });
    expect(request.send).toHaveBeenCalledOnce();
    expect(request.headers).toEqual(
      new Map([
        ["Accept", "text/event-stream"],
        ["Authorization", "Bearer device-token"],
        ["Cache-Control", "no-cache"],
      ]),
    );
    request.status = 200;
    request.readyState = 3;
    request.responseText = 'event: channel\ndata: {"channelId":"seven"}';
    request.onreadystatechange?.(new Event("readystatechange"));
    expect(frames).not.toHaveBeenCalled();
    request.responseText += "\n\n";
    request.onreadystatechange?.(new Event("readystatechange"));
    expect(frames).toHaveBeenCalledWith({ data: '{"channelId":"seven"}' });

    stream.close();
    stream.close();
    expect(request.aborted).toBe(true);
  });

  it("cancels initial connection work when closed before native request creation", () => {
    const timers = timerHarness();
    const createRequest = vi.fn(() => new RequestStub());
    const stream = createNativeEventStream("http://loomarr.test/v1/events", {
      clearTimer: timers.clearTimer,
      createRequest,
      setTimer: timers.setTimer,
    });

    stream.close();
    timers.callbacks.shift()?.();

    expect(timers.clearTimer).toHaveBeenCalledOnce();
    expect(createRequest).not.toHaveBeenCalled();
  });

  it("fails closed on revoked credentials and reconnects other terminal failures", () => {
    const timers = timerHarness();
    const requests: RequestStub[] = [];
    const onUnauthorized = vi.fn();
    const stream = createNativeEventStream("http://loomarr.test/v1/events", {
      clearTimer: timers.clearTimer,
      createRequest: () => {
        const request = new RequestStub();
        requests.push(request);
        return request;
      },
      onUnauthorized,
      reconnectMs: 123,
      setTimer: timers.setTimer,
    });

    timers.callbacks.shift()?.();
    requests[0]!.status = 500;
    requests[0]!.readyState = 4;
    requests[0]!.onreadystatechange?.(new Event("readystatechange"));
    expect(timers.setTimer).toHaveBeenLastCalledWith(expect.any(Function), 123);
    timers.callbacks.shift()?.();
    requests[1]!.status = 403;
    requests[1]!.readyState = 4;
    requests[1]!.onreadystatechange?.(new Event("readystatechange"));
    expect(onUnauthorized).toHaveBeenCalledOnce();
    expect(requests[1]!.aborted).toBe(true);
    expect(timers.callbacks).toHaveLength(0);

    stream.close();
  });

  it("reconnects a completed successful response without redispatching old bytes", () => {
    const timers = timerHarness();
    const requests: RequestStub[] = [];
    const stream = createNativeEventStream("http://loomarr.test/v1/events", {
      createRequest: () => {
        const request = new RequestStub();
        requests.push(request);
        return request;
      },
      setTimer: timers.setTimer,
    });
    const frames = vi.fn();
    stream.addEventListener("channel", frames);

    timers.callbacks.shift()?.();
    requests[0]!.status = 200;
    requests[0]!.readyState = 4;
    requests[0]!.responseText = 'event: channel\ndata: {"channelId":"one"}\n\n';
    requests[0]!.onreadystatechange?.(new Event("readystatechange"));
    timers.callbacks.shift()?.();
    requests[1]!.status = 200;
    requests[1]!.readyState = 3;
    requests[1]!.responseText = 'event: channel\ndata: {"channelId":"two"}\n\n';
    requests[1]!.onreadystatechange?.(new Event("readystatechange"));

    expect(frames.mock.calls.map(([event]) => event.data)).toEqual([
      '{"channelId":"one"}',
      '{"channelId":"two"}',
    ]);
    stream.close();
  });

  // As an EventSource does: "open" once for each connection the server accepts, so a caller hears
  // the stream come back and reads again what it missed while it was down.
  it("announces each connection the server accepts, once, and none it refuses", () => {
    const timers = timerHarness();
    const requests: RequestStub[] = [];
    const stream = createNativeEventStream("http://loomarr.test/v1/events", {
      createRequest: () => {
        const request = new RequestStub();
        requests.push(request);
        return request;
      },
      setTimer: timers.setTimer,
    });
    const opens = vi.fn();
    stream.addEventListener("open", opens);
    const answer = (request: RequestStub, status: number, readyState: number) => {
      request.status = status;
      request.readyState = readyState;
      request.onreadystatechange?.(new Event("readystatechange"));
    };

    timers.callbacks.shift()?.();
    answer(requests[0]!, 200, 3);
    answer(requests[0]!, 200, 3);
    expect(opens).toHaveBeenCalledOnce();
    answer(requests[0]!, 200, 4);
    timers.callbacks.shift()?.();
    answer(requests[1]!, 502, 4);
    expect(opens).toHaveBeenCalledOnce();
    timers.callbacks.shift()?.();
    answer(requests[2]!, 200, 3);
    expect(opens).toHaveBeenCalledTimes(2);
    stream.close();
  });

  // A native request keeps every byte of its response for as long as it lives, so a TV left on for
  // days would hold every frame it ever heard. Past its budget the connection is replaced at once;
  // the new connection's "open" is what tells the caller to read again what the swap could miss.
  it("replaces a connection that has carried its byte budget, without losing a frame", () => {
    const timers = timerHarness();
    const requests: RequestStub[] = [];
    const stream = createNativeEventStream("http://loomarr.test/v1/events", {
      createRequest: () => {
        const request = new RequestStub();
        requests.push(request);
        return request;
      },
      maxResponseBytes: 64,
      setTimer: timers.setTimer,
    });
    const frames = vi.fn();
    const opens = vi.fn();
    stream.addEventListener("channel", frames);
    stream.addEventListener("open", opens);

    timers.callbacks.shift()?.();
    const first = requests[0]!;
    first.status = 200;
    first.readyState = 3;
    first.responseText = 'event: channel\ndata: {"channelId":"one"}\n\n';
    first.onreadystatechange?.(new Event("readystatechange"));
    expect(first.aborted).toBe(false);
    first.responseText += 'event: channel\ndata: {"channelId":"two"}\n\n';
    first.onreadystatechange?.(new Event("readystatechange"));

    expect(frames.mock.calls.map(([event]) => event.data)).toEqual([
      '{"channelId":"one"}',
      '{"channelId":"two"}',
    ]);
    expect(first.aborted).toBe(true);
    // The replacement is immediate, not after the 5s a failure waits.
    expect(timers.setTimer).toHaveBeenLastCalledWith(expect.any(Function), 0);
    timers.callbacks.pop()?.();
    requests[1]!.status = 200;
    requests[1]!.readyState = 3;
    requests[1]!.onreadystatechange?.(new Event("readystatechange"));
    expect(opens).toHaveBeenCalledTimes(2);
    stream.close();
  });
});
