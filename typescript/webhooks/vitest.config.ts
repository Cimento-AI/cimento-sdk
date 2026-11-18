import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    // There is nothing to test until the SDK has code. Remove this with the
    // first test.
    passWithNoTests: true,
  },
});
