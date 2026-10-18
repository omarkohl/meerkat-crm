# Build stage
FROM --platform=$BUILDPLATFORM node:26-alpine AS builder

WORKDIR /app

# Copy package files first for better caching
COPY package.json package-lock.json ./

# Install dependencies
RUN npm ci

# Copy source code
COPY . .

# Build the app with the API URL injected at build time
ARG VITE_API_URL
ENV VITE_API_URL=${VITE_API_URL}

RUN npm run build

# Production stage - serve with unprivileged nginx
FROM nginxinc/nginx-unprivileged:1.30-alpine AS runtime

# Pick up Alpine security fixes published after the base image was built
# (the Trivy job in docker-build.yml fails on fixable HIGH/CRITICAL findings).
USER root
RUN apk upgrade --no-cache
USER 101

# Copy custom nginx config
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY security-headers.conf /etc/nginx/security-headers.conf

# Copy built assets from builder
COPY --from=builder /app/build /usr/share/nginx/html

# nginx-unprivileged runs on port 8080 by default
EXPOSE 8080

# Start nginx
CMD ["nginx", "-g", "daemon off;"]
