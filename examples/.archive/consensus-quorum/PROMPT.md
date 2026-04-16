# Consensus & Quorum Protocol

Voting workflows where **acting without quorum is a type error**.

## Proof Chain

```
voter-eligible ──┐
vote-unique ─────┼──► valid-vote (eligible + no double-vote)
cast-vote ───────┘          │
                       vote-tally (counted)
                            │
                  quorum-reached (total ≥ required)
                       │           │
              proposal-approved  proposal-rejected
                       │
              execution-authorized (can only execute approved)
```

## Key Invariants

- Cannot vote twice on the same proposal
- Cannot act without quorum
- Cannot execute a rejected proposal
- Veto requires mandatory reason
- Voter eligibility proven before vote accepted

## Use Cases

- Governance: DAO-style proposals and voting
- Approvals: budget, deployment, access grants
- Code review: require N approvals before merge
- Change management: change advisory board votes

See `specs/core.shen` for the full specification.
