const base64 =
  /^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/;

/**
 * Decodes standard base64 with its padding, and nothing looser. `atob` alone
 * would also take text without padding or with spaces in it, and so accept a
 * secret or a signature the Go SDK refuses.
 */
export function decodeBase64(text: string): Uint8Array<ArrayBuffer> | null {
  if (!base64.test(text)) {
    return null;
  }
  const binary = atob(text);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

export function encodeBase64(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary);
}

/** A request body: text, or the bytes as they arrived. */
export type Body = string | Uint8Array | ArrayBuffer;

export function bytesOf(body: Body): Uint8Array<ArrayBuffer> {
  if (typeof body === "string") {
    return new TextEncoder().encode(body);
  }
  // A copy, so the bytes are on an ArrayBuffer whatever the view was on.
  return new Uint8Array(body instanceof ArrayBuffer ? body : body.slice());
}

const rfc3339 =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|[+-](\d{2}):(\d{2}))$/;

/**
 * Whether text is an RFC 3339 time, by the rules the Go SDK parses with.
 * `Date.parse` is looser: it takes the 30th of February and 24:00.
 */
export function isTime(text: string): boolean {
  const match = rfc3339.exec(text);
  if (!match) {
    return false;
  }
  const [year, month, day, hour, minute, second, offsetHour, offsetMinute] =
    match.slice(1).map((part) => (part === undefined ? 0 : Number(part)));
  const leap = year! % 4 === 0 && (year! % 100 !== 0 || year! % 400 === 0);
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  return (
    month! >= 1 &&
    month! <= 12 &&
    day! >= 1 &&
    day! <= days[month! - 1]! &&
    hour! <= 23 &&
    minute! <= 59 &&
    second! <= 59 &&
    offsetHour! <= 23 &&
    offsetMinute! <= 59
  );
}
