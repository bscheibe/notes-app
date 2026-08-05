# syntax=docker/dockerfile:1

FROM golang:1.26 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/notes-app ./cmd/server

# distroless has no shell, so the notes directory is created here (with the
# nonroot image's fixed UID:GID) and copied over, rather than mkdir'd at
# runtime - which would fail with permission denied under USER nonroot.
RUN mkdir -p /var/lib/notes-app && chown 65532:65532 /var/lib/notes-app

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

COPY --from=build /out/notes-app ./notes-app
COPY config.prod.yaml ./config.prod.yaml
COPY templates ./templates
COPY --from=build --chown=65532:65532 /var/lib/notes-app /var/lib/notes-app

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["./notes-app"]
CMD ["-config", "config.prod.yaml"]
