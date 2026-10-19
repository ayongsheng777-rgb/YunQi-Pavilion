# Stage 1: Build Frontend
# 注：用 node:20-bookworm（NAS 本地已缓存，避免从 Docker Hub 拉取超时）
FROM node:20-bookworm AS frontend-builder

# 1. 接收构建参数（代理地址 + 镜像源）
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NPM_REGISTRY

# 2. 设置环境变量
ENV HTTP_PROXY=$HTTP_PROXY \
    HTTPS_PROXY=$HTTPS_PROXY \
    NPM_CONFIG_REGISTRY=${NPM_REGISTRY:-https://registry.npmjs.org}

WORKDIR /app/frontend
# 说明：Vite 的 publicDir 是 frontend/public（图标等静态素材源），
# 构建产物输出到 frontend/dist，最终镜像再拷到 server/public。
# 仓库里的 server/public 是旧构建输出，已不再提交，不需要 COPY。

# Copy package files first to cache dependencies
COPY frontend/package.json frontend/package-lock.json ./

# Install dependencies
RUN npm ci --prefer-offline --no-audit --timeout=300000

# Copy source code
COPY frontend/ .

# Build frontend (使用 server/public 作为 publicDir，与上方 COPY 一致)
ENV TAILWIND_DISABLE_NATIVE=1
ENV VITE_DOCKER_BUILD=1
RUN npm run build-only

# Stage 2: Build Backend
# 注：用 golang:1.23-bookworm（NAS 本地已缓存）；go.mod 要求 go1.25.5 时
# Go 工具链会自动经 GOPROXY 下载对应版本（已配 goproxy.cn）
FROM --platform=$BUILDPLATFORM golang:1.23-bookworm AS backend-builder

# 接收构建参数
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG GOPROXY=https://proxy.golang.org,direct

ENV HTTP_PROXY=$HTTP_PROXY \
    HTTPS_PROXY=$HTTPS_PROXY \
    GOPROXY=$GOPROXY \
    GOTOOLCHAIN=auto

WORKDIR /app/backend

# Copy go mod files
COPY backend/go.mod backend/go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY backend/ .

# Build binary
# Use ARG TARGETOS and TARGETARCH to support cross-compilation
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o yunqi-pavilion .

# Stage 3: Final Image
FROM alpine:latest

WORKDIR /app

# Install necessary runtime dependencies
# tzdata is important for correct timezone handling
RUN apk --no-cache add ca-certificates tzdata

# 设置时区和 Gin 模式
ENV TZ=Asia/Shanghai \
    GIN_MODE=release \
    BASE_DIR=/app

# Copy backend binary
COPY --from=backend-builder /app/backend/yunqi-pavilion .

# Copy frontend dist to public directory
# This includes the built assets and the static files copied from server/public during build
COPY --from=frontend-builder /app/frontend/dist ./server/public

# Create necessary directories for volumes
RUN mkdir -p server/data server/music server/PC server/APP server/doc server/icon-cache

# Expose port
EXPOSE 3000

# Run the application
CMD ["./yunqi-pavilion"]
