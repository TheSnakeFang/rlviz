# Use the workspace

The GUI is the only trajectory display. Collection, rollout, Detail, Guide, and Settings are movable modules. Additional rollouts open as rows by default.

## Smaller screens

At 1200px and wider, RLViz uses the full docked workspace. From 720px to 1199px, Collection stays beside one active module. Below 720px, use the module tabs and touch actions to view one module at a time. Resizing does not discard your open rollouts, selection, or desktop layout.

The compact view works on phones and narrow windows, but multi-rollout comparison, docking, and keyboard workflows are best at 1200px or wider.

## Collection

- `j` / `k` or arrow keys select a rollout and keep it in view.
- `Enter` opens the selected rollout. `a` adds it without replacing an open rollout.
- `[` / `]` change collection fidelity. The highest level names tool calls.
- Switch between rollout and trial grouping in the Collection header.

## Query indexed rollouts

The CLI viewer can query every indexed source from the Collection filter. Enter a whitespace-separated query, then press `Enter` or choose **query index**. Plain words perform a case-insensitive text search.

- Filter with `source:`, `run:`, `task:` (or `case:`), `group:`, `checkpoint:`, `model:`, `env:`, `status:`, `termination:`, `tool:`, and `pass:true` or `pass:false`.
- Bound numeric values with `reward>=`, `reward<=`, `tokens>=`, `tokens<=`, `cost>=`, and `cost<=`.
- Sort with `sort:reward`, `sort:tokens`, `sort:cost`, `sort:tools`, `sort:run`, `sort:case`, `sort:checkpoint`, or `sort:model`. Add `desc:true` for descending order.

For example: `run:eval-42 checkpoint:step-800 pass:false cost<=0.25 sort:reward desc:true`.

The indexed cohort readout reports matches and cost across the complete result, not only the current page. Choose **load more** to append the next page. The browser-only viewer filters its currently opened file locally and does not expose indexed queries.

## Local labels and descriptions

Open the collection or trajectory header editor to add a title, description, or comma-separated trajectory labels. These annotations are device-local browser data: they are searchable in the local Collection filter, survive reloads, and never modify the source trace. They are not indexed server filters or shared cohort metadata.

## Rollout and detail

- `j` / `k` move event by event. `e` jumps to the next error and `r` to the next reward or grader.
- `Enter` opens the selected section in place: overview, episode, events, then source. `Escape` goes back up. An unpinned side Detail closes as you descend so the same event is not shown twice.
- `d` opens or closes the shared Detail module. `Shift+D` pins it to the current rollout; `Shift+C` compacts or expands it.
- Drag the timeline window to pan, click to center it, or drag either edge to resize it.

## Modules and shortcuts

- `Tab` / `Shift+Tab` cycle modules. `Alt` plus an arrow activates the spatial neighbor.
- `Ctrl+m` toggles module move mode. `Ctrl+w` toggles seam resize mode. The same chord or `Escape` exits.
- `?` toggles Guide. `Shift+S` toggles Settings.
- The bottom bar shows the active module's current shortcuts. Guide includes the full default keybinding reference.
