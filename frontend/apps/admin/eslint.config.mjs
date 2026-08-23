import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";
import reactHooks from "eslint-plugin-react-hooks";

export default defineConfig([
  ...nextVitals,
  ...nextTs,
  {
    // Same relaxed React Compiler-era rules as the webmail app; keep them
    // visible as warnings until the codebase migrates to the new style.
    plugins: { "react-hooks": reactHooks },
    rules: {
      "react-hooks/set-state-in-effect": "warn",
      "react-hooks/immutability": "warn",
    },
  },
  globalIgnores([
    ".next/**",
    "node_modules/**",
    "**/node_modules.old/**",
    "next-env.d.ts",
    "public/**",
    "coverage/**",
  ]),
]);
