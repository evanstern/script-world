package tui

// Decision-trace projection (spec 020, TASK-63): a bounded per-agent record
// of "why did my villager do that" built incrementally from the same event
// stream applyEvent already folds into the replica and the chronicle ring
// (tui.go), joining cog.thought / cog.tool_call / cog.outcome on their
// shared job ID (research D1, contracts/decision-trace-ui.md §1). Pure
// event-sourced logic — no lipgloss, no Model — in the grammar.go/digest.go
// style; villagerDecisionsBody (views.go) renders it.
