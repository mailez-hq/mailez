# syntax=docker/dockerfile-upstream:1.4.3

# Build context is the frontend monorepo root (compose sets it), because the
# npm workspaces lockfile lives there and @mailez/types + @mailez/ui are
# workspace dependencies of this app (they export plain TS sources, so Next
# transpiles them — no pre-build step needed).
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
COPY apps/webmail/package.json apps/webmail/
COPY packages/types/package.json packages/types/
COPY packages/ui/package.json packages/ui/
RUN npm ci
COPY . .
RUN npm run build --workspace apps/webmail

# Runtime stage. node_modules is copied whole for robustness; switch to Next.js
# `output: "standalone"` later for a smaller image.
FROM node:22-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=3000
COPY --from=build /app ./
EXPOSE 3000
CMD ["npm", "run", "start", "--workspace", "apps/webmail"]
