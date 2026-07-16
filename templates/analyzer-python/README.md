# example-analyzer

This template is a dependency-free local RolloutViz analyzer. Implement stable
findings and derived signals in `analyzer.py`, then validate it with:

```sh
rlviz plugin trust .
rlviz plugin validate . sample-input.json
```
