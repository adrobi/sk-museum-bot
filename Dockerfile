FROM golang:1.26-alpine AS builder
WORKDIR /app

# Копируем зависимости
COPY go.mod go.sum ./ 
RUN go mod download

# Копируем исходники
COPY . .
# Собираем бинарник
RUN go build -o main .

FROM alpine:3.18
WORKDIR /app
# Копируем только бинарный файл из образа сборщика
COPY --from=builder /app/main .

# Файл .env НЕ копируем, он прокинется через docker-compose
CMD ["./main"]