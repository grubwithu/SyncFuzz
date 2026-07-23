# Dedicated runtime image for V2 synthesis profiling. SyncFuzz launches this
# image as an unprivileged, capability-dropped target container; eBPF remains
# on the host and filters only the container cgroup.
FROM python:3.11-slim

WORKDIR /opt/syncfuzz-langgraph

RUN apt-get update \
    && apt-get install -y --no-install-recommends bash \
    && rm -rf /var/lib/apt/lists/*

COPY targets/langgraph_shell_react/requirements.txt ./requirements.txt
RUN pip install --no-cache-dir -r requirements.txt

COPY targets/langgraph_shell_react/run_target.py ./run_target.py

RUN useradd --create-home --uid 10001 syncfuzz
USER syncfuzz
