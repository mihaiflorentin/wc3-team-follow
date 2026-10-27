# syntax=docker/dockerfile:1

# Build stage: compile a static binary from the root main package.
FROM golang:1.27-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/wc3-team-follow .

# Runtime stage: minimal image containing only the binary.
# Configuration is embedded; override any key with environment variables (e.g. HTTP_PORT).
FROM gcr.io/distroless/static-debian12:nonroot

# The binary embeds Go's timezone database (time/tzdata), so TZ resolves
# even though the distroless image ships no zoneinfo files of its own.
ENV TZ=Europe/Bucharest

COPY --from=build /out/wc3-team-follow /usr/local/bin/wc3-team-follow

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/wc3-team-follow"]
CMD ["server"]
