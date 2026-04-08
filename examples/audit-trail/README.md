# Immutable Audit Trail & Compliance

Hash-chained audit logs where **tampering is a type error**.

## Proof Chain

```
audit-entry (id + actor + action + entity + timestamp + sensitivity + chain-link)
       │
chain-continuous (prev hash = next's prev-hash + time-ordered)
       │
within-retention (entry age < retention period)
       │
compliance-report (complete chain + entry count > 0)
       │
chain-verified (all hashes recomputed and valid)
```

## Key Invariants

- Every entry links to previous via hash chain
- Entries must be time-ordered (no backdating)
- Compliance reports require unbroken chains
- Retention policies enforced at type level
- Reading audit logs requires elevated role
- Tamper detection via hash chain verification

## Action Types

`create | read | update | delete | approve | reject | escalate`

## Sensitivity Levels

`public | internal | confidential | restricted`

## Use Cases

- SOX/HIPAA/GDPR compliance logging
- Financial transaction audit trails
- Access logging for sensitive systems
- Change tracking with tamper detection

See `specs/core.shen` for the full specification.
