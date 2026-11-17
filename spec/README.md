# Formal Specifications

This folder holds formal models of Gorums components.
Each subfolder holds one model, its TLC configurations, and a README that explains the model and its results.

| Model             | Scope                                                                                                                                                         |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [stream](stream/) | The stream layer for one peer pair under stream deduplication: reconnects, the shared node's failover, flow control, and the delivery and ordering contracts. |

## Tools

The TLA+ models need Java and `tla2tools.jar`.
`spec/tlc.sh` takes the jar from `$TLA2TOOLS`, or else from the [TLA+ VS Code extension](https://marketplace.visualstudio.com/items?itemName=tlaplus.vscode-ide).
The jar is also on the [TLA+ releases page](https://github.com/tlaplus/tlaplus/releases).

## Running the Checks

Run the commands from the repository root.
List the configurations of every model, with their run time, expected result, and description:

```bash
just spec-list
```

Check one configuration by its name; `just tlc` shows TLC's progress, then the result and any error trace, one line per step:

```bash
just tlc Current
```

If two models have a configuration with the same name, name the model too, as in `just tlc stream/Current`.
Check every short configuration against its expected result:

```bash
just spec-check
```

Add the long configurations, which take up to half an hour:

```bash
just spec-check -all
```

Without `just`, run `spec/tlc.sh list`, `spec/tlc.sh run Current`, and `spec/tlc.sh check`.
TLC writes its output to `out/` next to each configuration; git ignores it.
To summarize saved output again, run `spec/tlc.sh summary Current`.

## Configuration Headers

Each `.cfg` file starts with three comment lines that `spec/tlc.sh` reads:

```text
\* Module: Stream
\* Expect: violates SlotTracksLive
\* Run: short
```

* `Module`: the module that TLC checks.
* `Expect`: `pass`, `deadlock`, or `violates P`, where `P` is an invariant or a temporal property.
* `Run`: `short` for under a minute, or `long`.

The first other comment line is the description that `just spec-list` shows.

A new model goes in its own subfolder, with these headers in each configuration and a README that explains the model.
