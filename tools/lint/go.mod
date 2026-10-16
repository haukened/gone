// Not Go code. This file marks tools/lint as a separate module so that
// `go test ./...`, `go vet ./...` and golangci-lint in the repository root
// skip it: npm installs tools/lint/node_modules here, and one of ESLint's
// dependencies (flatted) ships a Go package.
module github.com/haukened/gone/v3/tools/lint

go 1.27.2
