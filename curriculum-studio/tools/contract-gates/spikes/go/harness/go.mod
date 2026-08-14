// Nested module boundary: keeps spike.local/gen + grpc imports out of the
// production curriculum-studio module so `GOWORK=off go mod tidy -diff` stays clean.
// Full require/replace graph is materialised by run_spikes.sh into SPIKE_OUT.
module spike.local/harness

go 1.25.0
