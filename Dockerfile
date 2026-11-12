# ---- Этап 1: сборка ----
FROM golang:1.27-alpine AS build
WORKDIR /src

# Сначала только go.mod/go.sum: слой зависимостей кэшируется,
# и пересборка без изменений в зависимостях не качает модули заново
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO не нужен: modernc.org/sqlite — чистый Go. Статические бинарники.
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/api      ./cmd/api \
    && go build -trimpath -ldflags="-s -w" -o /out/worker   ./cmd/worker \
    && go build -trimpath -ldflags="-s -w" -o /out/notifier ./cmd/notifier

# ---- Этап 2: рантайм ----
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 appuser

# Каталог для SQLite: том монтируется сюда, и владелец — appuser,
# иначе процесс от appuser не сможет создать файл БД (SQLITE_CANTOPEN 14)
RUN mkdir -p /data && chown appuser:appuser /data

WORKDIR /app
COPY --from=build /out/  /app/
COPY configs/            /app/configs/

ENV TZ=Europe/Moscow
USER appuser
ENTRYPOINT ["/bin/sh", "-c"]
CMD ["/app/api"]
