# Contributing

Use Go 1.23 or newer. Run `go run .` to start the dashboard from your checkout. The browser assets in `web/` are embedded at build time.

Work on a branch, format changed Go files with `gofmt`, and run `go vet ./...` and `go test ./...` before opening a pull request. For runtime integration changes, name the operating system and runtime version you tested.
