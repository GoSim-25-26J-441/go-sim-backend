# -------- build stage --------
FROM golang:1.25.1-alpine AS build
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
WORKDIR /app

# Cache modules
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN go build -v -o /app/app ./cmd/api

# -------- runtime stage --------
FROM alpine:3.20
RUN apk add --no-cache graphviz ca-certificates
WORKDIR /app

# Only the binary (keep image small)
COPY --from=build /app/app /app/app

ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/app/app"]
