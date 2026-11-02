# Independent ACL oracle (P3-3)

Run with Python 3.13 and a separately built CLI:

```text
python -m pip install -r scripts/oracle/requirements.txt
go build -o bin/plenilune ./cmd/plenilune
python -m unittest discover -s scripts/oracle -p "test_*.py"
python scripts/oracle/differential.py --binary bin/plenilune
```

On Windows append `.exe` to the binary path. The CI workflow
`.github/workflows/oracle.yml` runs exactly this gate on pull requests and main.
No Python/Z3 dependency is part of Go runtime or release binaries.

The generator owns synthetic rule tuples. One projection emits IOS text to the
CLI; another builds full-width bitvector predicates and nested first-match
if-then-else expressions in Z3, with implicit deny. It does not import Go IR,
parser, packetset or verifier results to construct the oracle. Non-TCP/UDP
ports are constrained to zero, matching the declared complete-header scope.
Unsigned comparisons are essential for port boundaries (see the official
[Z3 bitvector documentation](https://microsoft.github.io/z3guide/docs/theories/Bitvectors/)).

The fixed seed includes equal pairs, shadowing, implicit deny, reordered rules,
terminal permits, TCP/UDP/numeric protocols, contiguous and non-contiguous
wildcards, and all numeric port operators including 0 and 65535. Both verdict
categories must occur. All emitted engine witnesses are evaluated using the
independent expression; a satisfying SMT packet is checked via CLI trace in
both directions. Solver unknown, engine incomplete, CLI error and disagreement
fail rather than count as agreement. Negative controls inject a false verdict
and forged witness actions to check that the gate rejects them.

Failures preserve exact synthetic before/after inputs, optional SMT packet,
seed and case index under ignored `out/oracle-failure/`; CI uploads that folder.
Investigate disagreements on both sides. Agreement is regression evidence on
this generated corpus, not a proof of engine correctness or device semantics.

Local result 2026-11-02, Windows, Python 3.13, Z3 4.13.4, seed 3304:
164 pairs passed (66 EQUIVALENT, 98 DIFFERENT). Hosted CI execution remains
unmeasured until the workflow is published by the owner.
