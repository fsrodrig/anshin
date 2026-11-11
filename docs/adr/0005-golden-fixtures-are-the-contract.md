# Golden fixtures are the API contract, not OpenAPI

The contract between Agent and phone is the set of JSON fixtures in `contract/fixtures/`, produced by the Agent's own tests from scrubbed `/proc` and `/sys` captures of real Machines and decoded by the phone's production decoder in its tests, with `contract/README.md` as the field glossary. The brief asked for an OpenAPI spec, but for one read-only endpoint a hand-written spec is a third copy that drifts, while fixtures emitted by the real code path cannot drift from what the Agent sends. CI regenerates them and fails on any diff; old fixtures are kept to prove backward compatibility, and a hand-written "future" fixture proves the phone tolerates additive changes.

## Considered Options

- **OpenAPI + generated Go models and phone validators**: real codegen, but two generator toolchains and a spec that can still disagree with behaviour.
- **Protobuf**: compact and generated, but not readable with curl and adds toolchains for a few kilobytes per Pulse.
