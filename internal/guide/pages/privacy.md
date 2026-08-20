# Privacy, storage, and limits

## Does RLViz upload traces?

No. The browser reads local files through the File API and holds its index in memory. The CLI binds to `127.0.0.1`, stores a removable SQLite index in its own state directory, and makes no outbound request during normal viewing.

`rlviz bundle create` is also local-only. It requires explicit review and redaction confirmations, a title, a license, and a new `.rlviz` destination. It refuses to overwrite. Opening a bundle verifies its canonical trace digest. An optional expiration in a portable file is advisory; a future hosted service must enforce link expiration and deletion separately.

A public link containing both `bundle` and `sha256` is an explicit fetch boundary. The browser first shows the HTTPS origin, path, and digest without contacting that host. If you choose **verify and open**, it makes one credential-free, no-referrer request, refuses redirects, enforces the browser size ceiling, and verifies the complete file digest before parsing. Local files are never uploaded.

Package installation and opening external documentation are separate network actions. Local process plugins run with the current user's permissions, which is why trust is explicit and digest-bound.

## What changes the source?

Nothing. Titles, descriptions, shortcuts, theme, named workspace state, and dock geometry are presentation state. They do not rewrite canonical or raw records.

## Which surface should I use?

Use the browser for individual supported files, reviewed `.rlviz` bundles, and modest cohorts. Use the CLI for complete Harbor job directories, bundle creation, larger or growing sources, private process adapters, persistent indexing, structured agent queries, and remotely controlled named workspaces.

## Where can an agent read these docs?

Run `rlviz guide --json`, fetch `https://rlviz.dev/llms.txt`, or read `https://rlviz.dev/llms-full.txt`. All three describe the same current product boundary and workflows as this Guide.
