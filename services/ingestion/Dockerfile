FROM golang:1.26.2 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/ingestion ./services/ingestion/cmd

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/ingestion /app/ingestion

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/app/ingestion"]
