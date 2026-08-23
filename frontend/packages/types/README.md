# @mailez/types

Shared wire types for the mailez admin and webmail apps.

## Sources

- `src/contract/*.ts` — curated, ergonomic types used by the apps (they mirror
  the backend JSON 1:1, with a few view-only fields such as `MailMessage.id`).
- `src/generated.ts` — machine-generated from the backend OpenAPI spec
  (`backend/docs/swagger.json`).

## Regenerating the generated types

The backend publishes its spec at `backend/docs/swagger.json` (produced by
`swag init`, served live at `/swagger/`). Regenerate the TypeScript contract
with:

```sh
npm run gen:types
```

The pipeline converts the Swagger 2.0 document to OpenAPI 3
(`src/openapi.json`, gitignored) and runs `openapi-typescript`.

## Roadmap

The curated contract files will progressively alias the generated schemas so
the backend OpenAPI document becomes the single source of truth.
