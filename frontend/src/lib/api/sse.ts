export type SSEEventHandler = (
  eventName: string,
  payload: Record<string, unknown>,
) => void;

/**
 * Consumes a fetch response body as server-sent events. Comment-only blocks
 * (heartbeats) are ignored; an `error` event throws so existing translation
 * streams retain their current failure behavior.
 */
export async function readSSEStream(
  response: Response,
  onEvent: SSEEventHandler,
): Promise<void> {
  if (!response.body) {
    throw new Error("Empty response stream");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder("utf-8");
  let buffer = "";

  const dispatchEventBlock = (rawBlock: string) => {
    const normalized = rawBlock.replace(/\r\n/g, "\n");
    let eventName = "message";
    const dataLines: string[] = [];

    for (const line of normalized.split("\n")) {
      if (!line || line.startsWith(":")) continue;
      if (line.startsWith("event:")) {
        eventName = line.slice(6).trim();
      } else if (line.startsWith("data:")) {
        dataLines.push(line.slice(5).trimStart());
      }
    }

    if (dataLines.length === 0) return;
    const payload = JSON.parse(dataLines.join("\n")) as Record<
      string,
      unknown
    >;
    if (eventName === "error") {
      throw new Error(
        typeof payload.error === "string" ? payload.error : "Stream failed",
      );
    }
    onEvent(eventName, payload);
  };

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });

      let boundary: number;
      while ((boundary = buffer.search(/\r?\n\r?\n/)) !== -1) {
        const match = buffer.slice(boundary).match(/^\r?\n\r?\n/);
        const sepLen = match ? match[0].length : 2;
        const block = buffer.slice(0, boundary);
        buffer = buffer.slice(boundary + sepLen);
        if (block.trim()) {
          dispatchEventBlock(block);
        }
      }
    }

    buffer += decoder.decode();
    if (buffer.trim()) {
      dispatchEventBlock(buffer);
    }
  } finally {
    reader.releaseLock();
  }
}
