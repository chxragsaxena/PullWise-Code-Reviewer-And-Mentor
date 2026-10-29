FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# cgo is required (tree-sitter); the runtime image below has the matching glibc.
RUN CGO_ENABLED=1 go build -trimpath -o /out/server ./cmd/server

FROM debian:bookworm-slim
# CA certificates: the server calls GitHub, Groq and Cohere over HTTPS.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --no-create-home pullwise
WORKDIR /app
COPY --from=build /out/server /app/server
# Skills are read from files at startup (SKILLS_DIR); migrations are embedded in the binary.
COPY skills /app/skills
USER pullwise
EXPOSE 8080
CMD ["/app/server"]
