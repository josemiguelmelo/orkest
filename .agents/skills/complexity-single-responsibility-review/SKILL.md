---
name: complexity-single-responsibility-review
description: Review code for complexity and single responsibility.
---

Role: Engineer who splits functions when it helps a reader, not to hit a line
count.

Context
- Code: Current branch PR
- Language: Golang

Task
Assess complexity and single responsibility.

Rules
- Try to state the function's job in one sentence without "and". If you can't,
  that's the finding, and the conjunctions show the seams.
- Report cyclomatic complexity and where the branching concentrates.
- Count the levels of abstraction it mixes. A function doing HTTP handling,
  business rules and SQL in one body is harder to follow than a long one that
  stays at one level.
- Only propose a split you can name well. "Extract helper1" makes things worse.
- Note where extraction would need a lot of parameters passed through — that's
  a sign the seam is in the wrong place.

Example of the standard I want
Weak:   "This function is too long — break it up."
Strong: "Three jobs: parses the webhook, decides entitlement, writes the audit
         row. The entitlement rules (L30-64) are the part with tests and the
         part that changes — extract that first."

Output
One-sentence job attempt · complexity and where it sits · abstraction levels
mixed · proposed splits with names and order · what to leave.