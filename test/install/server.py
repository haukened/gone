"""Serves a release directory the way GitHub serves releases, for
test/install/run.sh: /releases/latest redirects to /releases/tag/<tag>, and
/releases/download/<tag>/<file> serves <dir>/<file>.

    python3 -I server.py <dir> <tag> <port>
"""
import http.server
import os
import sys


def handler(root, tag):
    """Returns a request handler class serving the release in root as tag."""

    class Releases(http.server.BaseHTTPRequestHandler):
        """Answers GitHub's release URLs from a local directory."""

        def do_HEAD(self):  # pylint: disable=invalid-name
            """Answers a HEAD request without a body."""
            self.respond(body=False)

        def do_GET(self):  # pylint: disable=invalid-name
            """Answers a GET request."""
            self.respond(body=True)

        def respond(self, body):
            """Redirects /releases/latest, or serves one release file."""
            if self.path == "/releases/latest":
                self.send_response(302)
                self.send_header("Location", f"/releases/tag/{tag}")
                self.end_headers()
                return
            if self.path == f"/releases/tag/{tag}":
                self.send_response(200)
                self.end_headers()
                return
            prefix = f"/releases/download/{tag}/"
            name = self.path[len(prefix):] if self.path.startswith(prefix) else ""
            files = set(os.listdir(root))
            if name not in files:
                self.send_error(404)
                return
            with open(os.path.join(root, name), "rb") as release_file:
                data = release_file.read()
            self.send_response(200)
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            if body:
                self.wfile.write(data)

        def log_message(self, format, *args):  # pylint: disable=redefined-builtin
            """Keeps the test output quiet."""

    return Releases


def main():
    """Serves the release until killed."""
    root, tag, port = sys.argv[1], sys.argv[2], int(sys.argv[3])
    server = http.server.ThreadingHTTPServer(("127.0.0.1", port), handler(root, tag))
    server.serve_forever()


if __name__ == "__main__":
    main()
