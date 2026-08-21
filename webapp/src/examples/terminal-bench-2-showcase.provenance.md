# Terminal-Bench 2.0 reviewed cohort

`terminal-bench-2-showcase.ndjson` contains four public trajectories selected
from [`yoonholee/terminalbench-trajectories`](https://huggingface.co/datasets/yoonholee/terminalbench-trajectories/tree/04e8940f5b6736a7ce8d22224fe2f2af74163ed2)
at immutable revision `04e8940f5b6736a7ce8d22224fe2f2af74163ed2`
under Apache-2.0:

| Source row | Task | Agent | Reward | Reviewed bundle SHA-256 |
| ---: | --- | --- | ---: | --- |
| 242 | `adaptive-rejection-sampler` | `mini-swe-agent` | 0 | `3fc8dc4ab29664c629777fcdbb46de42c8eee4944ec4d1d1790417aab1eacaa1` |
| 244 | `adaptive-rejection-sampler` | `mini-swe-agent` | 1 | `a60152d61c996b47329708e48601a102e9ccf7549ec5882c29e2d9f061faac01` |
| 40384 | `qemu-startup` | `terminus-2` | 0 | `f36ad76baa42a7dead2bf1cf0247b07e8886b43e9ee0e71c757559dd0984c8ce` |
| 40385 | `qemu-startup` | `terminus-2` | 1 | `a38b2273d2a9ae7845574456285c9af17345dbe1bd14ca7bab69d9df2909cd9f` |

The task definitions are pinned to
[Terminal-Bench revision `2fd12b88aafdd04a52c298e3940bcb189f9766d6`](https://github.com/harbor-framework/terminal-bench-2/tree/2fd12b88aafdd04a52c298e3940bcb189f9766d6).
The four source bundles and their
review decisions are maintained in the
[RLViz benchmark catalog](https://github.com/TheSnakeFang/rlviz-benchmarks/tree/d9da3dc8a43f8f83160e1bf3088c6adb514f6f03).

The cohort composition preserves source records and event order. It emits the
shared run, cases, and groups once; scopes event, signal, and artifact IDs by
trajectory; updates their internal references; and copies each row and bundle
identity onto its trajectory. The resulting NDJSON SHA-256 is
`5570d196d1d40de16c2f174665d9649779e0a649a3ba7016078e5d94a17bcbd3`.
