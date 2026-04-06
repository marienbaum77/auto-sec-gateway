# Этап 1: Сборка (используем легкий образ Go)
FROM golang:1.22-alpine AS builder
WORKDIR /app
# Копируем файлы зависимостей
COPY go.mod go.sum ./
RUN go mod download
# Копируем код и собираем бинарник
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o manager ./main.go

# Этап 2: Финальный образ (минимальный размер)
FROM alpine:latest
WORKDIR /root/
# Копируем только скомпилированный файл
COPY --from=builder /app/manager .
# Открываем порт
EXPOSE 8080
# Запуск
CMD ["./manager"]