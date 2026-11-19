# CLI output parity

Run `bash rust/scripts/cli-parity.sh` from any directory. Requires Go, Rust
1.89.0, and Python 3 (standard library only). Binaries and persistent, isolated
run artifacts are stored under `rust/target/`. No keyring, real credential, or
external model service is needed. All model turns use `e2e-mock.py`.

The first phase compares both clients against Go; the second seeds with Go,
then reopens that database with Rust and repeats the comparisons. The stored
provider URL is set offline before Rust starts, since Go uses
`ANTHROPIC_BASE_URL` but Rust uses the provider record.

Each case records arguments, exit code, stdout, and stderr in JSON, with a
unified diff for mismatches. UUIDs are replaced **only for successful create
outputs**, because the two invocations create independent resources. No other
normalization is performed: field order, whitespace, types, timestamps, ANSI,
and error text remain visible. Successful creates are removed before repeating
name-unique operations; task/message mutations use independent tasks.

The default mode is an inventory and exits successfully after completing all
cases, even with differences. Pass `--strict` to fail on any difference,
including cosmetic error usage/help differences. Setup failures always fail.

## Coverage

* modelprovider/model/agent/task/message: list and get in default, table, JSON,
  YAML, card and Markdown; successful create/delete; missing get/delete, bad
  positional arguments, unknown flags and invalid formats.
* Message create also exercises all six output modes (Go accepts the render
  flags but returns only the ID); empty message lists exercise all six modes.
* skill list, context list, info and daemon token list: all six output modes.
* config list/get, context current, missing required create flags, duplicate
  create and unreachable daemon.

The suite is not exhaustive for administrative commands: skill install/delete
with installed fixtures, authenticated token create/revoke, config/context
mutations, daemon lifecycle/deployment, and agent apply are not covered.
No network installation or token enrollment is attempted. Wide/no-header table
variants are covered by the existing Rust renderer/command tests rather than
this subprocess matrix. This suite compares clients on each server, not Go
server responses directly against Rust server responses.

## Intentional compatibility choices

Go writes create IDs to **stderr**, despite successful status. Rust now does the
same for all five core resources. Scripts should capture stderr explicitly when
using those commands. Go's message display concatenates text parts only; tool
calls/results/errors are absent from `content` and do not add table columns.
Rust follows that behavior even though exposing them might be more useful.

Remaining error differences are not normalized: Cobra prints command usage
after RPC errors, whereas Rust prints only the error; Clap and Cobra use
different argument-error wording and usage formatting. Both clients return 1
and write errors to stderr. Unreachable-daemon messages also retain their
different transport-library diagnostics. These are text differences, not
successful JSON/YAML or exit-status differences.

## Verified result

Final run on both daemon implementations:

```text
SUMMARY: 216/272 match; 56 differ
```

Each daemon has 136 cases, 108 exact matches and 28 stderr-text differences.
There are no exit-status mismatches. The successful output matrix is:

| Commands | Default | Table | JSON | YAML | Card | Markdown |
| --- | --- | --- | --- | --- | --- | --- |
| modelprovider list/get | match | match | match | match | match | match |
| model list/get | match | match | match | match | match | match |
| agent list/get | match | match | match | match | match | match |
| task list/get | match | match | match | match | match | match |
| message list/get/create | match | match | match | match | match | match |
| message empty list | match | match | match | match | match | match |
| skill list | match | match | match | match | match | match |
| context list | match | match | match | match | match | match |
| info | match | match | match | match | match | match |
| daemon token list | match | match | match | match | match | match |

All five core resource creates/deletes match in their supported default mode.
Config list/get and context current match. Missing-resource, bad-argument,
unknown/missing-flag, invalid-format, duplicate-create and unreachable errors
differ in stderr text but match exit status and stream routing.
