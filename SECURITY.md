# Security policy

The Agent answers network requests on the Machines it watches, so its security matters more than its size suggests. Thank you for looking.

## Reporting a vulnerability

Report privately through GitHub: [open a draft security advisory](https://github.com/fsrodrig/anshin/security/advisories/new). Please don't open a public issue, pull request or discussion for a vulnerability.

Include the version, what you did, what happened and what you expected. Leave out anything that identifies your own Machines: hostnames, tailnet names or addresses, MAC addresses, serial numbers, usernames, and raw `/proc` or `/sys` dumps. A scrubbed reproduction is enough.

This is a one-person project, so there is no response-time promise. Reports are read, confirmed fixes are released as a new version, and reporters are credited in the advisory unless they ask not to be.

## Supported versions

Anshin is pre-1.0. Only the latest release gets fixes.

## In scope

- Any request that the Agent's authorization truth table should refuse but doesn't, or that it lets through when tailscaled's LocalAPI fails ([design](docs/design.md#agent-side)).
- The Agent listening on any address other than its own tailnet addresses.
- Any path by which the Agent writes, executes, kills or configures anything on its Machine. The API is read-only by design.
- The phone app connecting to an address outside the tailnet ranges, including through a system proxy or a redirect ([design](docs/design.md#phone-side)).
- The phone app sending a credential, or anything about the phone, to an Agent.
- A release artifact that doesn't match the source it claims to be built from.

## Out of scope

The accepted risks in the design's [residual risks](docs/design.md#residual-risks), such as a carrier-grade NAT address that is not on the tailnet showing fake numbers.
