FROM golang:latest-alpine
WORKDIR /app
COPY . .
RUN go mod tidy
RUN go build -o /Meower-Data-Exporter
ENTRYPOINT ["/Meower-Data-Exporter"]