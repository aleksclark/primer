# C3 qualification spikes

Executable hard-type qualification for Curriculum Studio generators.

```bash
# from curriculum-studio/
make contracts-spikes
# or
./tools/contract-gates/spikes/run_spikes.sh
```

## Layout

```text
spikes/
  fixtures/           # retained JSON fixtures (conformance seeds)
  evidence/           # last-run digests and shape evidence (committed)
  go/harness/         # nested module spike.local/harness (isolated from production go.mod)
  generate_twice.sh   # deterministic proto + openapi-typescript generation
  run_spikes.sh       # full gate
  REPORT.md           # PROCEED/STOP per shape
  .tmp/               # gitignored build-only outputs
```

## Rules

- No business materializer
- Generated stubs only under `.tmp/` (gitignored)
- STOP if a required generator cannot represent a shape without hand DTOs
- Ordinary `make contracts-spikes` regenerates committed `evidence/` + `REPORT.md` **byte-stably** (no wall-clock stamps; fixed semantic fixture timestamps). CI must be able to run the target twice with zero tracked diff.
- **Qualified generation path:** `buf generate` in an isolated temp copy of `contracts/{buf.yaml,buf.gen.yaml,proto}` using committed **local** pins (`local: protoc-gen-go` @ v1.36.11, `local: protoc-gen-go-grpc` @ v1.5.1) resolved by `contracts/scripts/bootstrap_local_plugins.sh` into an ignored private bin on `PATH`. BSR remote plugins and ambient host `protoc-gen-go` are **not** used. Digests and PROCEED are invalid if the path or pins differ, bootstrap fails, or remote plugins reappear.
- **RED (remote non-reproducibility):** `evidence/remote_rate_limit_red.txt` records the parent ordinary second-run BSR `resource_exhausted` failure that forced the local-plugin path.
