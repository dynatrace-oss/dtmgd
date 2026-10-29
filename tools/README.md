# tools

A separate Go module that pins the developer tools CI runs (today: govulncheck), checksum-locked in `go.sum` and kept current by Dependabot. It is not part of the dtmgd build.

Run govulncheck with `make vulncheck`, or: `go -C tools build -o bin/govulncheck golang.org/x/vuln/cmd/govulncheck && bin/govulncheck ./...`
