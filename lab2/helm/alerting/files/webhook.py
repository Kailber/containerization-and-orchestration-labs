"""Print received Alertmanager notifications to stdout for the lab report."""

import json
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/health":
            self.send_error(404)
            return
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"ok\n")

    def do_POST(self):
        if self.path != "/alerts":
            self.send_error(404)
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
            if not 0 < length <= 1024 * 1024:
                self.send_error(413)
                return
            payload = json.loads(self.rfile.read(length))
            if not isinstance(payload, dict) or not isinstance(payload.get("alerts"), list):
                self.send_error(400)
                return
        except (ValueError, UnicodeDecodeError):
            self.send_error(400)
            return

        print(json.dumps({
            "time": datetime.now(timezone.utc).isoformat(),
            "event": "alert_notification",
            "status": payload.get("status"),
            "receiver": payload.get("receiver"),
            "alerts": payload["alerts"],
        }, ensure_ascii=False), flush=True)
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"received\n")

    def log_message(self, format, *args):
        # Keep stdout limited to notifications, without health probe noise.
        pass


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
