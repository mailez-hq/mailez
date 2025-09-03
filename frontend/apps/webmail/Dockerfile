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
# Edition is a BUILD-TIME decision: next.config.ts materializes edition/
# from ce-stubs/ or ee/ and bakes NEXT_PUBLIC_MAILEZ_EDITION into the
# client bundle. A runtime env cannot change it afterwards. Default ce;
# enterprise builds pass --build-arg MAILEZ_EDITION=ee (see compose).
ARG MAILEZ_EDITION=ce
# API_TARGET is equally BUILD-TIME: Next bakes rewrite destinations into
# routes-manifest.json, so the compose runtime env cannot re-point them.
# Default matches the compose network alias the browser-facing proxy must
# reach; local host builds keep using `npm run dev` with its own default.
ARG API_TARGET=http://backend:8080
RUN MAILEZ_EDITION=${MAILEZ_EDITION} API_TARGET=${API_TARGET} npm run build --workspace apps/webmail

# Runtime stage. node_modules is copied whole for robustness; switch to Next.js
# `output: "standalone"` later for a smaller image.
FROM node:22-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=3000
COPY --from=build /app ./
EXPOSE 3000
CMD ["npm", "run", "start", "--workspace", "apps/webmail"]
