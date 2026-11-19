/**
 * A plain SMTP server for the platform e2e tests. The platform server sends its
 * notices through it (SMTP_HOST pointing here), so a test can read the message
 * the user would receive. It offers no STARTTLS and no AUTH, which the mailer accepts
 * for a relay on 127.0.0.1. Messages are kept in memory for the life of the test file.
 */
import net from "node:net";

export interface SentMail {
  to: string;
  data: string;
}

export interface FakeSmtp {
  port: number;
  sent: SentMail[];
  close(): Promise<void>;
}

export function startFakeSmtp(): Promise<FakeSmtp> {
  const sent: SentMail[] = [];
  const sockets = new Set<net.Socket>();
  const server = net.createServer((socket) => {
    sockets.add(socket);
    socket.on("close", () => sockets.delete(socket));
    socket.on("error", () => sockets.delete(socket));
    let buffer = "";
    let rcpt: string[] = [];
    let inData = false;
    let dataLines: string[] = [];
    socket.write("220 fake-smtp ready\r\n");
    socket.on("data", (chunk) => {
      buffer += chunk.toString("utf8");
      let idx: number;
      while ((idx = buffer.indexOf("\r\n")) >= 0) {
        const line = buffer.slice(0, idx);
        buffer = buffer.slice(idx + 2);
        if (inData) {
          if (line === ".") {
            inData = false;
            const data = dataLines.join("\r\n");
            for (const to of rcpt) sent.push({ to, data });
            rcpt = [];
            dataLines = [];
            socket.write("250 queued\r\n");
          } else {
            dataLines.push(line.startsWith("..") ? line.slice(1) : line);
          }
          continue;
        }
        const verb = line.split(" ")[0].toUpperCase();
        if (verb === "EHLO" || verb === "HELO") {
          socket.write("250-fake-smtp\r\n250 OK\r\n");
        } else if (verb === "MAIL") {
          socket.write("250 OK\r\n");
        } else if (verb === "RCPT") {
          const m = /<([^>]*)>/.exec(line);
          rcpt.push((m ? m[1] : line.slice(8)).trim());
          socket.write("250 OK\r\n");
        } else if (verb === "DATA") {
          inData = true;
          socket.write("354 end with <CRLF>.<CRLF>\r\n");
        } else if (verb === "RSET" || verb === "NOOP") {
          rcpt = [];
          socket.write("250 OK\r\n");
        } else if (verb === "QUIT") {
          socket.end("221 bye\r\n");
        } else {
          socket.write("502 not implemented\r\n");
        }
      }
    });
  });
  return new Promise((resolveStart, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (!address || typeof address === "string") {
        reject(new Error("fake SMTP did not bind a port"));
        return;
      }
      resolveStart({
        port: address.port,
        sent,
        close: () =>
          new Promise<void>((done) => {
            for (const s of sockets) s.destroy();
            server.close(() => done());
          }),
      });
    });
  });
}
