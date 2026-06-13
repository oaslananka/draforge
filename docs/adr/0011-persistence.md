# ADR-0011: Persistence Strategy

- **Status**: Deferred (Stateless for v0.1.0)
- **Context**: We need a way to store historical demo information or TUI/web configuration if needed, without running heavy databases.
- **Decision**: Keep the system entirely stateless for v0.1.0 by reading live Kubernetes objects. SQLite persistence for optional historical demo tracking is deferred to a future milestone.
- **Alternatives**: Running PostgreSQL or MySQL on the cluster (destabilizes small node size).
- **Consequences**: Lightweight, fast, low resource overhead.
- **Security Considerations**: SQLite database file is encrypted on disk.
- **Operational Considerations**: High reliability and trivial backup.
