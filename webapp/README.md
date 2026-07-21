# RLViz browser viewer

This is the static application deployed at `app.rlviz.dev`, separate from the
documentation site at `rlviz.dev`. It reuses the instrument viewer in
`web/src` through the `ViewerProvider` interface and supplies an in-memory Go
WebAssembly provider.

Build from the repository root:

```bash
make webapp
python3 -m http.server 8000 --directory webapp/dist
```

`webapp/dist` is a complete static output containing the JavaScript and CSS
bundle, `rlviz.wasm`, the matching local `wasm_exec.js`, and `vercel.json`. It has no CDN or
runtime package dependency and can be uploaded directly as a Vercel static
output.

Trace files are read through the browser `File` API. Raw trace and adapter bytes
are never sent in a request. The Go core and UI enforce the same 32 MiB ceiling;
larger files are directed to the streaming CLI before parsing.

The deployed policy is `default-src 'self'; connect-src 'self'; frame-ancestors
'none'` plus narrow runtime allowances. `script-src 'self' 'wasm-unsafe-eval'`
is required for `WebAssembly.compile`/Go WASM, `worker-src 'self'` permits only
the bundled adapter worker, `style-src 'self' 'unsafe-inline'` permits React's
numeric lane geometry, and `img-src 'self' blob: data:` permits local artifact
previews. No remote origin is allowed. Vercel also emits `X-Frame-Options:
DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, and
`Cross-Origin-Opener-Policy: same-origin`.
