# ADR-0011: Persistence Strategy

- **Status**: Approved
- **Context**: We need a way to store historical demo information or TUI/web configuration if needed, without running heavy databases.
- **Decision**: Keep the system primarily stateless by reading live Kubernetes objects. For optional historical demo tracking, use a small block storage volume with SQLite.
- **Alternatives**: Running PostgreSQL or MySQL on the cluster (destabilizes small node size).
- **Consequences**: Lightweight, fast, low resource overhead.
- **Security Considerations**: SQLite database file is encrypted on disk.
- **Operational Considerations**: High reliability and trivial backup.
