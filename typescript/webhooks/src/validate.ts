import { isTime } from "./encoding";
import { payloadShapes } from "./shapes.gen";

export function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/**
 * Whether data fits a payload: every field the payload has is there, none is
 * null unless it may be, and each is of the right kind. Fields the payload
 * does not have are ignored, and so is whether a value is one of a field's
 * known values, so nothing Cimento adds makes data stop fitting.
 */
export function fits(data: unknown, payload: string): boolean {
  const shape = payloadShapes[payload];
  if (shape === undefined || !isObject(data)) {
    return false;
  }
  return shape.every(([key, kind, nullable]) => {
    if (!Object.hasOwn(data, key)) {
      return false;
    }
    const value = data[key];
    if (value === null) {
      return nullable;
    }
    switch (kind) {
      case "string":
        return typeof value === "string";
      case "time":
        return typeof value === "string" && isTime(value);
      case "integer":
        return Number.isInteger(value);
      case "strings":
        return (
          Array.isArray(value) &&
          value.every((item) => typeof item === "string")
        );
      default:
        return fits(value, kind.payload);
    }
  });
}
