# Phase 1 Data Model: Metatron Instruction Surface

## 1. Instruction files (on-disk, player-editable)

| Entity | Path | Cardinality | Caps | Fallback (each with notice) |
|---|---|---|---|---|
| Charter | `<worldDir>/charter.md` | exactly 1 (restored if missing) | 4,000 chars (`persona.CharterMaxChars`) | missing → restore default; empty → default; oversize → truncate — UNCHANGED (`loadCharter`) |
| Skill file | `<worldDir>/skills/<name>.md` | 0..8 composed | 4,000 chars each | oversize → truncate; >8 files → skip beyond 8th in sort order; unreadable → skip |
| Capability manifest | `<worldDir>/capabilities.json` | 0..1 | — | missing → full default grant, NO notice; malformed → full default grant + notice; unknown names → ignored + notice |

Skill-file eligibility: regular files, `.md` extension, direct children of `skills/`
(no recursion); anything else silently excluded (not a notice — `.DS_Store` noise).
Composition order: ascending bytewise lexicographic filename order.

**Read discipline**: every file above is read fresh at each use site (each `Turn()`, each
`Status()`), no watcher, no cache — an edit is in effect at the next read (FR-001).

## 2. Composed system prompt (in-memory, per turn)

```
<charter effective text>                 ← editable
--- skill: <filename> ---                ← 0..8 blocks, editable, sort order
<skill effective text>
...
--- (fixed frame, beneath the charter and skills) ---
<fixed frame>                            ← compile-time Go constant, ALWAYS LAST
  ├─ the two non-negotiables (verbatim, unchanged)
  ├─ doctrine prose (judge-first, one act/turn, refusal free, no gratis, no villager removal)
  └─ tool.MetatronToolGuidance(grantedRoster)   ← derived, granted subset only
```

Invariants:
- **INV-1**: the fixed frame is appended unconditionally after all editable content on
  every code path; per-file truncation happens before assembly, so no editable byte can
  displace or truncate it (spec FR-003, SC-002).
- **INV-2**: identical world dir ⇒ byte-identical composed prompt (FR-012).
- **INV-3**: guidance text is a pure function of the granted roster (FR-008).

## 3. Capability manifest (parsed)

```go
// internal/metatron (parse target; not persisted beyond the read)
type manifest struct {
    Tools        []string `json:"tools"`
    MiracleKinds []string `json:"miracle_kinds"` // nil/omitted = all kinds
}
```

Validation → effective grant set:
- `Tools` filtered to names in `tool.LoopRosterMetatron()` (unknown → notice, ignored).
- `MiracleKinds` filtered to `tool.MiracleKinds()` (unknown → notice, ignored).
- `converse` never appears (it is not a roster tool; granting/denying it is meaningless —
  conversation is always on, FR-006).
- Absent file ⇒ grant = full `LoopRosterMetatron()` + all kinds (FR-007).
- `tools: []` (present, empty) ⇒ grant = nothing; conversation-only world.

## 4. Granted roster (in-memory, per turn)

```
grantedRoster := filter(tool.LoopRosterMetatron(), manifest.Tools)
if work_miracle granted && manifest.MiracleKinds != nil:
    work_miracle = tool.RestrictEnum(work_miracle, "kind", manifest.MiracleKinds)
```

Consumers (all three layers, R5):
1. `toolloop.Job.Roster` — declaration (schema-level structural absence).
2. `tool.MetatronToolGuidance(grantedRoster)` — prose.
3. `turnHandlers` built from granted set only + `landNudge`/`landMiracle` grant check —
   door. Reducer dry-run (`sim.spendMiracleCharge` etc.) unchanged as final authority.

State note: charges are world state, not capability — revoking a tool mid-world leaves
the bank intact but unspendable through that tool (edge case, spec).

## 5. Miracle cost single source (internal/tool)

```go
// registry.go — beside miracleKinds; THE authoritative table.
// kind → charges; event-type mapping declared alongside.
miracleCosts = map[string]int{"move":1, "remove":1, "give_item":1, "time_snap":2}
kindToEvent  = {"move":"metatron.entity_moved", "remove":"metatron.entity_removed",
                "give_item":"metatron.item_granted", "time_snap":"metatron.time_snapped"}

func MiracleCost(kind string) (int, bool)
func MiracleCostsByEvent() map[string]int   // fresh map per call
```

Derivations (replacing today's three copies):
- `sim.miracleCost` → `var miracleCost = tool.MiracleCostsByEvent()` (import exists).
- Prose costs → `MetatronToolGuidance` renders from `MiracleCost`.
- `work_miracle.Cost.Charges` stays 1 = gate minimum (documented as min(costs), not a
  price; unchanged semantics).

## 6. Extended Status (model-free peek; rides existing IPC JSON)

```go
type Status struct {
    Charges         int      `json:"charges"`
    CharterDefault  bool     `json:"charter_default"`
    SoulTail        string   `json:"soul_tail"`
    Skills          []string `json:"skills,omitempty"`           // NEW: effective files, composition order
    GrantedTools    []string `json:"granted_tools,omitempty"`    // NEW: e.g. "nudge_dream", "work_miracle(move,give_item)"
    ManifestDefault bool     `json:"manifest_default"`           // NEW: true when no capabilities.json
}
```

Old clients ignore unknown fields (encoding/json); no IPC protocol change.

## 7. State transitions

None persisted — every read is stateless recomputation from disk (the feature's whole
point). The only stateful artifact is unchanged: the charge bank in sim state.
