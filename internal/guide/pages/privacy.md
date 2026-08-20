# Privacy, storage, and limits

## Does RLViz upload traces?

No. The browser reads local files through the File API and holds its index in memory. The CLI binds to `127.0.0.1`, stores a removable SQLite index in its own state directory, and makes no outbound request during normal viewing.

Package installation and opening external documentation are separate network actions. Local process plugins run with the current user's permissions, which is why trust is explicit and digest-bound.

## What changes the source?

Nothing. Titles, descriptions, shortcuts, theme, named workspace state, and dock geometry are presentation state. They do not rewrite canonical or raw records.

## Which surface should I use?

Use the browser for individual supported files and modest cohorts. Use the CLI for complete Harbor job directories, larger or growing sources, private process adapters, persistent indexing, structured agent queries, and remotely controlled named workspaces.

## Where can an agent read these docs?

Run `rlviz guide --json`, fetch `https://rlviz.dev/llms.txt`, or read `https://rlviz.dev/llms-full.txt`. All three describe the same current product boundary and workflows as this Guide.
