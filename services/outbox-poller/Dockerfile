FROM golang:1.26.2 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/outbox-poller ./services/outbox-poller/cmd

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/outbox-poller /app/outbox-poller

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/app/outbox-poller"]
