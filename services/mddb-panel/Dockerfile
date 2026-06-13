# Development stage
FROM node:26-alpine AS development

WORKDIR /app

# Copy package files
COPY services/mddb-panel/package*.json ./

# Install all dependencies (including dev dependencies)
RUN npm install

# Copy source code
COPY services/mddb-panel/ .

# Copy shared scripts for build-time import
COPY scripts/mddb_model.py ./scripts/mddb_model.py

# Expose port
EXPOSE 3000

# Start dev server with hot reload
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]

# Builder stage
FROM node:26-alpine AS builder

WORKDIR /app

# Copy package files
COPY services/mddb-panel/package*.json ./

# Install dependencies
RUN npm ci

# Copy source code
COPY services/mddb-panel/ .

# Copy shared scripts for build-time import
COPY scripts/mddb_model.py ./scripts/mddb_model.py

# Build application
RUN npm run build

# Production image
FROM node:26-alpine AS production

WORKDIR /app

# Copy package files and production server
COPY services/mddb-panel/package*.json ./
COPY services/mddb-panel/server.js ./

# Install production dependencies only (express, http-proxy-middleware)
RUN npm ci --omit=dev

# Copy built application
COPY --from=builder /app/dist ./dist

# Expose port
EXPOSE 3000

# OPS-006: drop root — the official node image ships an unprivileged `node`
# user. The proxy binds the non-privileged port 3000, so no root is needed.
USER node

# OPS-006: liveness probe so compose can gate dependents (service_healthy) and
# restart a hung process.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q --spider http://localhost:3000/ || exit 1

# Start production server with reverse proxy
CMD ["node", "server.js"]
