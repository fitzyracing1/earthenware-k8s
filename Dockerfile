FROM golang:1.23 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /earthenware-scheduler ./cmd/scheduler
FROM gcr.io/distroless/static
COPY --from=build /earthenware-scheduler /earthenware-scheduler
ENTRYPOINT ["/earthenware-scheduler"]
