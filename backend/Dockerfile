FROM golang:1.26-alpine AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

FROM alpine:latest
WORKDIR /app

COPY --from=build /app/server .
COPY blacklist.txt .

EXPOSE 8080
CMD ["./server"]