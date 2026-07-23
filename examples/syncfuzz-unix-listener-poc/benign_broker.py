#!/usr/bin/env python3
"""A control-only Unix broker which never persists request payloads."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import select
import socket
import time
from pathlib import Path

from syncfuzz_root_cause_lib import utc_now, write_json


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--workspace", required=True)
    parser.add_argument("--duration", type=float, default=300)
    args = parser.parse_args()
    workspace = Path(args.workspace).resolve()
    socket_path = workspace / "branch-listener.sock"
    pid_path = workspace / "benign-broker-pid.txt"
    events_path = workspace / "benign-broker-events.jsonl"
    if socket_path.exists() or socket_path.is_symlink():
        raise SystemExit(f"refusing to replace existing control endpoint: {socket_path}")
    server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    server.bind(str(socket_path))
    server.listen(16)
    pid_path.write_text(str(os.getpid()), encoding="utf-8")
    write_json(
        workspace / "benign-broker-ready.json",
        {
            "pid": os.getpid(),
            "ready_at": utc_now(),
            "socket_path": str(socket_path),
            "role": "benign_broker",
        },
    )
    deadline = time.monotonic() + args.duration
    while time.monotonic() < deadline:
        readable, _, _ = select.select([server], [], [], 0.2)
        if not readable:
            continue
        connection, _ = server.accept()
        data = connection.recv(65536)
        # A trusted control service can receive a request but deliberately records
        # only a digest and byte length, never the test credential payload.
        event = {
            "received_at": utc_now(),
            "bytes": len(data),
            "payload_sha256": hashlib.sha256(data).hexdigest(),
        }
        with events_path.open("a", encoding="utf-8") as stream:
            stream.write(json.dumps(event, sort_keys=True) + "\n")
        connection.sendall(b"SYNCFUZZ_BENIGN_BROKER_ACCEPTED\n")
        connection.close()
    server.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
