# Benchmark: resolve PTR/reputation lookups via the stack's own resolver so
# per-message DNS time is deterministic (Docker's embedded DNS is slow).
dns {
  nameserver = "192.168.210.254";
  timeout = 2s;
}
