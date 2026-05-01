FROM golang:1.25-alpine AS build

WORKDIR /src
ARG VERSION=dev

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum* ./
RUN go mod download

COPY VERSION ./
COPY cmd ./cmd
COPY internal ./internal

RUN VERSION_VALUE="${VERSION}" \
    && if [ "${VERSION_VALUE}" = "dev" ] && [ -f VERSION ]; then VERSION_VALUE="$(cat VERSION)"; fi \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION_VALUE}" -o /out/lillian-backend ./cmd/backend

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -u 10001 backend

WORKDIR /app

COPY --from=build /out/lillian-backend /usr/local/bin/lillian-backend
COPY migrations ./migrations

ENV PORT=8787

EXPOSE 8787

USER backend

ENTRYPOINT ["lillian-backend"]
