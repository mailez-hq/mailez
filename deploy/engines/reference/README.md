# engines/reference

reference server mail engine (引擎 B) integration point.

When integrated, this directory will hold the engine component Dockerfile
(built and tagged by `backend/cmd/build-images`) and the corresponding
`docker-compose.reference.yml` override in `deploy/`.

Current status: evaluation complete, PoC planned (3-5 days). The PoC validates
the directory adapter, Management API provisioning (quota/app passwords) and
the authentication routing decision before a full 20-30 day integration.
See [`docs/engines/reference-evaluation.md`](../../docs/engines/reference-evaluation.md).
