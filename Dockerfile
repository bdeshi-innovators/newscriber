FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/webhook ./cmd/webhook

FROM alpine:3.20
RUN apk add --no-cache ca-certificates ffmpeg
WORKDIR /app
COPY --from=build /out/webhook /app/webhook
EXPOSE 8080
USER nobody
ENTRYPOINT ["/app/webhook"]
