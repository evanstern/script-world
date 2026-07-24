---
id: TASK-90
title: >-
  main broken: PR #58/#60 cross-merge missed situatedMemoryEvent signature
  change
status: Done
assignee: []
created_date: '2026-07-24 12:46'
updated_date: '2026-07-24 12:47'
labels: []
dependencies: []
priority: high
ordinal: 77000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
origin/main (e42383e) does not compile: spec 030 (PR #60) made situatedMemoryEvent take a required origin param (internal/sim/memory.go:189, provenance stamped at emission); spec 032 (PR #58) merged after it with two call sites written pre-030 — internal/sim/executor.go:712 (axe-broke memory) and internal/sim/executor.go:802 (built-a-wall memory) — passing 6 args. Both are executor action-outcome memories; the fix is OriginAction, matching every neighboring executor emission site (e.g. executor.go:113/329/381). Trivial exemption (constitution): surgical, file:line diagnosis complete, ACs here. Found while resolving PR #59's merge conflicts.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 go build ./... green on main
- [x] #2 both call sites pass OriginAction; full sim suite green
<!-- AC:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Fixed on main: both spec-032 call sites (executor.go axe-broke + built-a-wall memories) now pass OriginAction per the spec-030 provenance convention, matching every neighboring executor emission site. go build ./... green; full internal/sim suite green. Root cause: PR #58 merged after PR #60 without reconciling the new required origin param.
<!-- SECTION:FINAL_SUMMARY:END -->
