# Go for the Agent, not Rust

The Agent is written in Go (static, `CGO_ENABLED=0`, linux/amd64 and linux/arm64) even though Rust was on the table for being "blazing fast and lightweight". Rust would likely use less memory, but the difference is invisible on the Machines being watched; Go cross-compiles with no extra toolchain, needs only the standard library for HTTP and JSON, matches Tailscale's own ecosystem, and is where AI coding agents iterate fastest. We revisit only if the measured Agent misses its budgets (≤ 15 MB RSS, ≤ 0.1 % of one core idle) by a wide margin.
