import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

export default defineConfig([
  ...nextVitals,
  ...nextTs,
  {
    // The React Compiler-era hooks rules flag common dialog/load patterns
    // (resetting state when a panel opens, loading flags in effects) that are
    // still idiomatic here. Keep them visible as warnings until the codebase
    // is migrated to the compiler-friendly style.
    rules: {
      "react-hooks/set-state-in-effect": "warn",
      "react-hooks/immutability": "warn",
    },
  },
  globalIgnores([
    ".next/**",
    "node_modules/**",
    "next-env.d.ts",
    "public/**",
    "coverage/**",
  ]),
]);
