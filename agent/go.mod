module github.com/fsrodrig/anshin/agent

go 1.27.4

require golang.org/x/sys v0.50.0

require (
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	github.com/hexops/gotextdiff v1.0.3 // indirect
	github.com/tailscale/hujson v0.0.0-20260727124030-b80ff77dac4f // indirect
	github.com/tailscale/hujson/cmd/hujsonfmt v0.0.0-20260727124030-b80ff77dac4f // indirect
	golang.org/x/exp/typeparams v0.0.0-20231108232855-2478ac86f678 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/tools v0.51.0 // indirect
	honnef.co/go/tools v0.8.2 // indirect
)

tool (
	github.com/tailscale/hujson/cmd/hujsonfmt
	honnef.co/go/tools/cmd/staticcheck
)
