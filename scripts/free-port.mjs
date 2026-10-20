#!/usr/bin/env node
// Prints a free TCP port. With a start port it probes upwards from there, otherwise the OS picks one.
// Usage: node scripts/free-port.mjs [start]
import net from 'node:net';
import { pathToFileURL } from 'node:url';

export function freePort(start = 0) {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once('error', (err) => {
      if (err.code === 'EADDRINUSE' && start > 0) resolve(freePort(start + 1));
      else reject(err);
    });
    server.listen(start, () => {
      const { port } = server.address();
      server.close(() => resolve(port));
    });
  });
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  console.log(await freePort(Number(process.argv[2] ?? 0)));
}
