# =============================================================================
# KubeVirt Shepherd — Production Frontend Dockerfile
# =============================================================================
# Multi-stage build for Next.js production deployment.
#
# The project's next.config.ts uses require.resolve() in webpack config,
# which needs the build to run through the project's own build script
# (npm run build → run-next-build.mjs → next build --webpack).
# =============================================================================

# Stage 1: Install ALL dependencies (including devDependencies)
FROM node:22-alpine AS deps
WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

# Stage 2: Build the production bundle
FROM node:22-alpine AS builder
WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY . .

ENV NEXT_TELEMETRY_DISABLED=1
ENV NEXT_PUBLIC_API_URL=/api/v1
# NODE_ENV is set to production by the build script internally.
# We keep it unset here to allow devDependencies in node_modules to work for build tooling.

# Use the project's own build script which handles tsconfig and --webpack flag
RUN npm run build

# Stage 3: Prune back to production-only dependencies
FROM node:22-alpine AS prodrunner
WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci --omit=dev

# Stage 4: Production runtime
FROM node:22-alpine AS runner
WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

RUN addgroup --system --gid 1001 shepherd && \
    adduser --system --uid 1001 shepherd

# Copy production node_modules only
COPY --from=prodrunner /app/node_modules ./node_modules

# Copy package metadata for `next start`
COPY --from=builder /app/package.json ./package.json
COPY --from=builder /app/next.config.ts ./next.config.ts
COPY --from=builder /app/tsconfig.json ./tsconfig.json

# Copy built output
COPY --from=builder /app/.next ./.next
COPY --from=builder /app/public ./public

# Copy source files needed by next.config.ts at runtime (webpack vendor alias)
COPY --from=builder /app/src/vendor ./src/vendor

USER shepherd

EXPOSE 3000

CMD ["npx", "next", "start", "--hostname", "0.0.0.0", "--port", "3000"]
