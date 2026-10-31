# Frozen compatibility receipts

- comparison-v01.json / repair-v01.json: historical ACL v0.1 fixtures.
- analysis-routing-v01.json: original analysis of testdata/configs/routing pair,
  generated before prefix-list v0.2 hardening.
- analysis-prefix-v01.json + its before/after.cfg: original v0.1 analysis with
  implicit subset assumptions and sequence4294967295 (accepted by the old model,
  rejected by the current parser). This is historical replay coverage, **not**
  evidence that a Cisco device accepts the syntax.

Keep these files frozen. Do not regenerate them merely to make a new implementation
pass. Tests replay their stored digests and results with explicit version dispatch;
new analysis must not fall back to historical parsing on an unsupported input.
