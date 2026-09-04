---
name: Adapter request
about: Request or propose support for a new coding-agent adapter
labels: [adapter]
---

**Agent / platform**

Name and version of the coding agent or harness.

**Detected application/config locations**

Where does this agent store its configuration and session state?

**Capabilities**

What lifecycle hooks or integration points does it expose (begin, preflight,
checkpoint, finish, MCP, other)?

**Privacy classification**

What does this agent read or write that Overgent would need to observe? See
`docs/adapter-development.md` before proposing an implementation.
