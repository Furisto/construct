# Vendored protobuf dependencies

- `buf/validate/validate.proto`: bufbuild/protovalidate tag `v0.14.0`,
  `proto/protovalidate/buf/validate/validate.proto`.
- `google/type/decimal.proto`: googleapis/googleapis commit
  `5ce024537af135bb2a0d3228072dd660047956aa`, `google/type/decimal.proto`.

Both upstream files retain their Apache-2.0 license headers. Validation options
are parsed at build time but are not enforced by the generated Rust messages.
No `google/api` protos are currently imported by the Construct schema.
Google well-known protos are supplied by `protoc-bin-vendored`.
