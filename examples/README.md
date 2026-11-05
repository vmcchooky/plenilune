# Examples

Synthetic inputs only — every address below is a documentation range
(RFC 5737 / RFC 3849). Each directory holds a `before.cfg` / `after.cfg`
pair that runs through the configuration-level flow:

```sh
plenilune analyze --before examples/<dir>/before.cfg --after examples/<dir>/after.cfg
```

| Directory | Capability | Expected result |
|---|---|---|
| `standard-acl/` | Standard IPv4 ACL (P3-5) | `FILTER-IN` is `changed`: the added exact-host deny takes effect ahead of the subnet permit |
| `object-groups/` | Object-groups in named extended ACLs (P3-6) | `MGMT` is `changed`: the group gains a second web server; the rule stays one logical rule |
| `ipv6-acl/` | IPv6 ACL (P3-7) | `V6-EDGE` is `equivalent`: one /48 permit rewritten as two /49 permits |
| `cis/` | CIS obligation library (P3-8) | Obligation sets in `plenilune.obligations/v1` format; see `docs/cis-obligation-library.md` |
| `moonstorm-client/` | MoonStorm integration sample client | Go (stdlib only) subprocess client; see `docs/integration/moonstorm.md` |

Extended IPv4 ACL and prefix-list pairs also live as end-to-end
fixtures under `testdata/configs/` (regression, routing, refactor,
intended) and in the walkthroughs (`docs/walkthrough-en.md`,
`docs/walkthrough-vi.md`). The pairs above are pinned by
`internal/analysis/n3_examples_test.go`.
