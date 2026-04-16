# RBAC & Capability-Based Authorization

Authorization as a **proof chain**. Privilege escalation is a type error.

## Proof Chain

```
user-id ─► active-session (time-bounded)
                │
           role-binding (user has role in org)
                │
           capability (role permits action on resource-type)
                │
           access-grant (capability + same-org proof)
                │
           audit-entry (every access logged)
```

## Key Types

- **Roles**: `admin | editor | viewer | auditor` (set membership)
- **Resources**: `document | project | settings | billing | user`
- **Actions**: `create | read | update | delete | list`
- **Delegation**: time-bounded subset of grantor's capabilities

## Key Invariants

- Cannot construct admin capability without admin role binding
- Cannot access resources across organizations
- Sessions expire as type errors (not runtime checks)
- Every access produces an audit entry

See `specs/core.shen` for the full specification.
