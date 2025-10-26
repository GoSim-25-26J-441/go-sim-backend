# build
FROM golang:1.25.2-alpine AS build
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /app/app ./cmd/api

# runtime
FROM alpine:3.20
RUN apk add --no-cache graphviz ca-certificates
WORKDIR /app
COPY --from=build /app/app /app/app
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/app/app"]
