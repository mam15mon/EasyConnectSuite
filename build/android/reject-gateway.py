"""Local test gateway that explicitly rejects one password login."""
import http.server
import ssl
import sys

certificate, key, modulus, count_path = sys.argv[1:]

class Gateway(http.server.BaseHTTPRequestHandler):
    def reply(self, body):
        content = body.encode()
        self.send_response(200)
        self.send_header("Content-Length", str(len(content)))
        self.end_headers()
        self.wfile.write(content)

    def do_GET(self):
        if self.path.startswith("/por/login_auth.csp"):
            self.reply(f"<Auth><TwfID>0000000000000001</TwfID><RSA_ENCRYPT_KEY>{modulus}</RSA_ENCRYPT_KEY><RSA_ENCRYPT_EXP>65537</RSA_ENCRYPT_EXP><RndImg>0</RndImg></Auth>")
        else:
            self.send_error(404)

    def do_POST(self):
        self.rfile.read(int(self.headers.get("Content-Length", "0")))
        if self.path.startswith("/por/login_psw.csp"):
            with open(count_path, "a", encoding="utf-8") as count:
                count.write("rejected\n")
            self.reply("<Auth><Result>0</Result></Auth>")
        else:
            self.send_error(404)

    def log_message(self, format, *args):
        pass

server = http.server.HTTPServer(("127.0.0.1", 18444), Gateway)
context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.load_cert_chain(certificate, key)
server.socket = context.wrap_socket(server.socket, server_side=True)
server.serve_forever()
