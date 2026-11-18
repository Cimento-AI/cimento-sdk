import js from "@eslint/js";
import { defineConfig, globalIgnores } from "eslint/config";
import tseslint from "typescript-eslint";

export default defineConfig([
  globalIgnores(["**/dist/"]),
  js.configs.recommended,
  tseslint.configs.recommended,
  {
    // Scripts run by Node as they are. TypeScript knows the globals of the
    // files it checks; for these, eslint has to be told.
    files: ["**/scripts/*.mjs"],
    languageOptions: {
      globals: { URL: "readonly", console: "readonly", process: "readonly" },
    },
  },
]);
