# Klados server image — headless build, no CGO/Wails/GTK.

# ---- Proto codegen (gen/ and frontend/src/gen/ are gitignored) ----
FROM bufbuild/buf:latest AS protogen
WORKDIR /src
COPY buf.yaml buf.gen.yaml ./
COPY proto proto
RUN buf generate

# ---- Frontend ----
FROM node:25-alpine AS frontend
RUN npm install -g pnpm@10
WORKDIR /src
COPY pnpm-workspace.yaml pnpm-lock.yaml package.json ./
COPY frontend/package.json frontend/package.json
COPY packages packages
COPY apps/docs/package.json apps/docs/package.json
RUN pnpm install --frozen-lockfile
COPY frontend frontend
COPY schemas schemas
COPY --from=protogen /src/frontend/src/gen frontend/src/gen
RUN cd frontend && pnpm build

# ---- Backend ----
FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=protogen /src/gen gen
COPY --from=frontend /src/frontend/dist frontend/dist
RUN CGO_ENABLED=0 go build -tags headless -trimpath -ldflags "-s -w" -o /klados .

# ---- Runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /klados /klados
# All persistent state (imported kubeconfigs, config, session, plugins,
# schema cache) lives under /data — mount a PVC there.
ENV XDG_CONFIG_HOME=/data/config \
    XDG_STATE_HOME=/data/state \
    XDG_DATA_HOME=/data/data \
    XDG_CACHE_HOME=/data/cache \
    HOME=/data
EXPOSE 8080
ENTRYPOINT ["/klados", "serve", "--addr", ":8080"]
