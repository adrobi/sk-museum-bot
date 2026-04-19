# Build stage
FROM node:20-alpine AS builder

WORKDIR /app

ARG NPM_REGISTRY=https://registry.npmjs.org/

# Настройка npm для скорости и стабильности сети
RUN npm config set registry ${NPM_REGISTRY} && \
    npm config set fetch-retry-mintimeout 20000 && \
    npm config set fetch-retry-maxtimeout 120000 && \
    npm config set fetch-timeout 300000 && \
    npm config set prefer-offline false

COPY package.json package-lock.json ./

# Используем --mount для кэширования между билдами
RUN --mount=type=cache,target=/root/.npm \
    npm ci --legacy-peer-deps --no-audit --no-fund --progress=false

# Копирование исходников и сборка
COPY . .
RUN npm run build

# Production stage - только nginx + dist, очень маленький образ
FROM nginx:alpine

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
