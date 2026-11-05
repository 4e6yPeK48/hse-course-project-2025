# build stage
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /featureban .

# runtime stage
FROM alpine:3.22
WORKDIR /app
RUN addgroup -S featureban && adduser -S -G featureban featureban && mkdir data metrics && chown featureban:featureban data metrics
COPY --from=builder /featureban .
USER featureban
EXPOSE 8080
CMD ["./featureban"]
