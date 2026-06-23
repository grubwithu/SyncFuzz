# Examples

This directory is reserved for minimal PoC inputs and exported minimized testcases.

The first implemented testcase is generated directly by the Go runner:

```bash
go run ./cmd/syncfuzz run --case orphan-process --out runs
```

Future examples should include:

- testcase manifest;
- seed primitive;
- fault phase;
- expected mismatch signature;
- minimized reproduction command.

