---
id: TASK-51
title: Inventory and storage v1
status: In Progress
assignee: []
created_date: '2026-07-22 01:42'
updated_date: '2026-07-22 01:42'
labels: []
dependencies:
  - TASK-50
ordinal: 46000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Implement the storage layer specced in specs/013-inventory-storage (from TASK-26's design session): single bulk carry cap (24, everything costs 1), emergent ground piles + stockpile zones from a drop action (no player zoning), death drops, builder-owned finite chests (6 planks, 48 bulk), theft recorded-not-prevented via existing social machinery, food rot on the ground but not in chests. Reflex never touches storage; all storage goals are planner-only. Depends on TASK-50 (planks, fine-grained food).

Spec: specs/013-inventory-storage
<!-- SECTION:DESCRIPTION:END -->
