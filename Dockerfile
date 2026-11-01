
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/ ./cmd/streetworks ./cmd/killswitch

FROM gcr.io/distroless/static-debian12:nonroot AS killswitch
COPY --from=build /out/killswitch /killswitch
ENTRYPOINT ["/killswitch"]

FROM gcr.io/distroless/static-debian12:nonroot AS api
COPY --from=build /out/streetworks /streetworks
COPY streetworks.db /srv/streetworks.db
ENTRYPOINT ["/streetworks", "serve", "-db", "/srv/streetworks.db"]
