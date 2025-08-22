# syntax=docker/dockerfile-upstream:1.4.3

# Build stage: install dependencies and produce the production bundle.
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# Runtime stage. node_modules is copied whole for robustness; switch to Next.js
# `output: "standalone"` later for a smaller image.
FROM node:22-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=3000
COPY --from=build /app ./
EXPOSE 3000
CMD ["npm", "run", "start"]
