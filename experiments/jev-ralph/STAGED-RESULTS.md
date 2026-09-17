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

The manifest correction removed shell quoting from each regex argument. A
baseline check confirmed that all nine seeded failures participated in the
three behavioral gate groups before repair.

Two corrected paired runs were then executed, reversing arm order in the second
pair as pre-registered.

| Run | Arm | Passed | Repair turns | Selected gates | Pi seconds | Total Pi tokens | Pi cost |
|---|---|---:|---:|---|---:|---:|---:|
| control-first | control | yes | 3 | tenant, commerce, workflow | 99 | 20,013 | $0.001572 |
| control-first | JEV | yes | 3 | tenant, commerce, workflow | 81 | 39,909 | $0.001680 |
| JEV-first | control | yes | 3 | tenant, commerce, workflow | 74 | 20,383 | $0.002750 |
| JEV-first | JEV | yes | 3 | tenant, commerce, workflow | 137 | 23,028 | $0.000975 |

All four arms passed with zero immutable-contract violations and no scheduler
fallbacks. Both schedulers selected exactly the same sequence:

1. `tenant-contracts`
2. `commerce-contracts`
3. `workflow-contracts`

JEV's distribution confidence increased as the frontier contracted. Across the
two corrected pairs it selected tenant at 0.64–0.65, commerce at 0.93–0.94, and
workflow at 0.99. Each JEV arm used 8,152 JEV input tokens and 540 output tokens
over three calls. Combined JEV latency was 1.54s in the first pair and 3.64s in
the second.

## Corrected conclusion

Under the pre-registered interpretation rule, this is a **null result**. JEV did
not improve deterministic acceptance or turns to acceptance, and it did not
change the investigation order chosen by the simple first-failure scheduler.

Secondary measurements do not favor a stable story. Combined control Pi time
was 173 seconds versus 218 for JEV. Combined Pi-reported cost was approximately
$0.004322 for control versus $0.002655 for JEV, but cache-read composition and
provider sampling differed substantially. JEV also added 16,304 input and 1,080
output tokens outside Pi. These differences are exploratory and cannot override
the equal primary outcomes.

The result is still informative: for this gate topology, JEV reproduced the
deterministic manifest order rather than finding a better schedule. The next
useful comparison should not be JEV versus a deliberately simple scheduler on
the same homogeneous seed. It should introduce multiple defect seeds where
gate order has a known asymmetric value, and compare JEV against a
dependency-aware deterministic scheduler.
