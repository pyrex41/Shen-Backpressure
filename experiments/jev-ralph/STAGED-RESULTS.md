# Staged-frontier results

## Invalid pilot runs

Two staged pairs were run on 2026-09-17 before a manifest defect was detected.
The control arm selected `commerce-contracts` then `workflow-contracts`. JEV
selected the same sequence. One JEV arm happened to pass after one turn while
the other three arms required two turns.

These pilots are invalid for the intended benchmark. Shell quotes embedded in
the manifest's `go test -run` arguments became literal regex characters because
`sb` executes commands directly. The tenant/support tests and the first/last
alternatives in other groups were not executed. The apparent acceptance was
therefore incomplete.

The pilot also showed that a prompt-only staging constraint is porous: Luna
edited functions outside the selected failure domain in every audited arm.
That behavior is recorded as a threat to the staged manipulation even after
the gate selector is corrected.

## Corrected runs

Pending. The manifest correction removes shell quoting from each regex argument
so all nine seeded contract tests participate in acceptance.
