# Immutable image authority for the primer release set.
# Production selector is registry@sha256:<64 hex> only. Tags are trace metadata.
#
# Live baseline at contract introduction (source-only; not yet reconciled):
#   primer          trace_tag=f0ab751
#   primer-tv       trace_tag=6cd71b3
#   content-ingest  trace_tag=6cd71b3
# Digests are OCI index digests from GHCR (docker buildx imagetools inspect).

image_primer          = "ghcr.io/aleksclark/primer@sha256:53a0562581fb3d0992f49ad11383791f3ee5db824467cfa2d72bffe7a9a466f9"
image_primer_tv       = "ghcr.io/aleksclark/primer-tv@sha256:91e41bf8ef4cdda84ff57b53ddc216612a8537ed07c06d82f361d0e434362d6f"
image_content_ingest  = "ghcr.io/aleksclark/content-ingest@sha256:3b98bddaf8bda80d94f1d983d4698149090bd2d7bd1c49f5cca715059a0cd2f5"

# Optional human trace (never runtime selectors):
# image_primer_trace_tag         = "f0ab751"
# image_primer_tv_trace_tag      = "6cd71b3"
# image_content_ingest_trace_tag = "6cd71b3"
