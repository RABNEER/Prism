"""Small stdio MCP client for PRISM's executable acceptance checks."""
import json
import pathlib
import queue
import subprocess
import threading


class MCP:
    def __init__(self, server, data_dir, extra_args=(), timeout=200):
        self.process = subprocess.Popen([str(pathlib.Path(server).resolve()), "-data-dir", str(pathlib.Path(data_dir).resolve()), *extra_args], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, encoding="utf-8", bufsize=1)
        self.messages = queue.Queue()
        self.sequence = 0
        self.timeout = timeout
        threading.Thread(target=lambda: [self.messages.put(line) for line in self.process.stdout], daemon=True).start()
        self.initialized = self.request("initialize", {"protocolVersion": "2025-11-25", "capabilities": {}, "clientInfo": {"name": "prism-pipeline-verifier", "version": "0.3.0"}})
        self.process.stdin.write(json.dumps({"jsonrpc": "2.0", "method": "notifications/initialized"}) + "\n")
        self.process.stdin.flush()

    def request(self, method, params):
        self.sequence += 1
        self.process.stdin.write(json.dumps({"jsonrpc": "2.0", "id": self.sequence, "method": method, "params": params}) + "\n")
        self.process.stdin.flush()
        while True:
            message = json.loads(self.messages.get(timeout=self.timeout))
            if message.get("id") == self.sequence:
                if "error" in message:
                    raise RuntimeError(message["error"])
                return message["result"]

    def call(self, name, arguments):
        result = self.request("tools/call", {"name": name, "arguments": arguments})
        if result.get("isError"):
            raise RuntimeError("; ".join(c.get("text", "") for c in result.get("content", [])))
        return result["structuredContent"]

    def close(self):
        self.process.stdin.close()
        try:
            self.process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.process.kill()
            self.process.wait(timeout=5)
