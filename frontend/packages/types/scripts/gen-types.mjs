import { readFile, writeFile } from "node:fs/promises";

import { convertObj } from "swagger2openapi";

// Backend swag emits Swagger 2.0; convert to OpenAPI 3 for the
// openapi-typescript CLI, which runs next in `npm run gen:types`.
const swagger = JSON.parse(
  await readFile(new URL("../../../../backend/docs/swagger.json", import.meta.url), "utf8"),
);
const { openapi } = await convertObj(swagger, { patch: true });
await writeFile(new URL("../src/openapi.json", import.meta.url), JSON.stringify(openapi));
console.log("converted to src/openapi.json");
