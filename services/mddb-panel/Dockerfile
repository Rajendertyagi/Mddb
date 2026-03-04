# Development stage
FROM node:25-alpine AS development

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
FROM node:25-alpine AS builder

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
FROM node:25-alpine AS production

WORKDIR /app

# Copy package files
COPY services/mddb-panel/package*.json ./

# Install all dependencies (including vite for preview)
RUN npm ci

# Copy built application
COPY --from=builder /app/dist ./dist

# Expose port
EXPOSE 3000

# Start production preview server
CMD ["npm", "run", "preview", "--", "--host", "0.0.0.0", "--port", "3000"]
