# Install or open RLViz

## Start in the browser

Open `https://rlviz.dev`. The viewer loads four reviewed public Terminal-Bench trajectories: reward-0 and reward-1 runs for two tasks. Use Settings to open another local trace or reviewed `.rlviz` bundle, choose a clearly labeled synthetic example, change theme, or load a reviewed browser adapter.

Supported files are parsed in memory in the current tab. No account or upload is involved.

## Install the local CLI

Homebrew:

```sh
brew install TheSnakeFang/tap/rlviz
```

npm:

```sh
npm install --global rlviz
```

Verified shell installer:

```sh
curl -fsSL https://rlviz.dev/install.sh | sh
```

Then run the setup wizard and open a trace:

```sh
rlviz init
rlviz inspect ./path/to/trace.ndjson
rlviz open ./path/to/trace.ndjson
```

`rlviz` with no arguments restores the last usable source. With no history it opens the bundled synthetic gallery.

## Let a coding agent operate the workspace

Install the RLViz instructions during `rlviz init`, or run `rlviz setup agent`. The agent can query canonical trajectories, choose IDs, open a named GUI workspace, and update that open workspace without browser automation.
