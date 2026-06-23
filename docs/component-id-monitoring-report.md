---
date: 2026-06-23
status: monitoring
---

# Component ID Monitoring Report

## Summary

We saw a component notification like:

```text
Component: Embeddings
Status: unknown -> Operational
```

Current decision: monitor next incidents/component updates before changing storage behavior.

## Findings

- Component checkpoints are stored in Redis as `component_id -> last_status`.
- Redis key: `openai-status:component-statuses`.
- Component names are not stored in that checkpoint hash.
- Component names come from live OpenAI `summary.json` during each poll.
- The bot compares component status by `component.ID`, then formats the alert with `component.Name`.
- Incident `page_id` is not used to detect components.
- `page_id` identifies the OpenAI status page, not a specific component.
- `unknown -> Operational` means current component found, but no previous checkpoint existed for that component ID.

## Possible Problem

OpenAI/Statuspage may have changed, recreated, or migrated the component ID.

If component ID changed:

- Bot treats the component as new.
- Previous status becomes `unknown`.
- Old Redis component ID remains unused.
- Component-filtered subscriptions using old ID may stop matching.
- Unfiltered component subscriptions still receive updates.

Other possible causes:

- Redis checkpoint was cleared or partially lost.
- `openai-status:initialized` exists while `openai-status:component-statuses` is missing entries.
- OpenAI added a new component.

## Current State

- No code change now.
- Monitor upcoming incidents and component updates.
- Compare current OpenAI component IDs with stored Redis component IDs.
- Decide later whether ID-only storage is enough.

## Future Options

- Keep ID-based storage if OpenAI component IDs prove stable.
- Store component name alongside ID for easier debugging.
- Add name fallback/alias logic if component IDs change in practice.
- Make component-filtered subscriptions survive ID changes by matching name when ID missing.

## Unresolved Questions

- Did OpenAI actually change the component ID?
- Are old component IDs present in Redis but absent from current `summary.json`?
- Should component subscriptions survive ID changes by matching component name?
