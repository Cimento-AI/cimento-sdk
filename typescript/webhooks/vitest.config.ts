import { fileURLToPath } from "node:url";

import { defineConfig } from "vitest/config";

export default defineConfig({
  resolve: {
    // The examples import the package by name, as a reader's code would.
    alias: {
      "@cimento/webhooks": fileURLToPath(
        new URL("./src/index.ts", import.meta.url),
      ),
    },
  },
});
