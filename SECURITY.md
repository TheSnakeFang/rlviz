# Security policy

RLViz reads traces that may contain private code, prompts, credentials, customer data, and local filesystem paths.

Do not report suspected vulnerabilities through a public issue. Use GitHub's private vulnerability reporting for this repository.

## Security expectations

- The viewer binds to loopback by default.
- Source traces are read-only.
- Normal viewing performs no outbound network requests.
- Recorded commands and tools are never re-executed.
- External plugins require explicit trust.
- Trace-provided file paths do not automatically grant filesystem access.

These are product invariants. Changes affecting them require explicit review and security tests.

## Plugin boundary

RLViz core opens source traces read-only and never replays recorded tools.
External adapters and analyzers are executable programs, not data-only
extensions. Once explicitly trusted, they run with the invoking user's operating
system permissions; RLViz does not provide an OS sandbox. Review plugin code
and dependencies before trusting it, and run untrusted community plugins inside
your own container or sandbox. Any content change invalidates RLViz trust
and requires approval again.
