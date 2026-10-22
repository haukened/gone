'use strict';

// A static server for the built website (site/public), standing in for
// GitHub Pages in the site tests: index.html for directories, 404.html with a
// 404 status for anything missing, and nothing else. Node built-ins only.

const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');

const root = path.resolve(process.env.SITE_DIR || path.join(__dirname, '../../../site/public'));
const port = Number(process.env.SITE_PORT || 8299);

const types = {
  '.html': 'text/html; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.json': 'application/json',
  '.xml': 'application/xml',
  '.txt': 'text/plain; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.ico': 'image/x-icon',
  '.woff2': 'font/woff2',
  '.sh': 'text/x-shellscript; charset=utf-8',
  '.yaml': 'text/yaml; charset=utf-8',
};

// resolve maps a request path to a file under root, or null.
function resolve(urlPath) {
  let p = decodeURIComponent(urlPath.split('?')[0]);
  if (p.endsWith('/')) p += 'index.html';
  const file = path.join(root, path.normalize(p));
  if (!file.startsWith(root + path.sep)) return null;
  if (fs.existsSync(file) && fs.statSync(file).isDirectory()) return path.join(file, 'index.html');
  return fs.existsSync(file) ? file : null;
}

function send(res, status, file) {
  res.writeHead(status, { 'Content-Type': types[path.extname(file)] || 'application/octet-stream' });
  fs.createReadStream(file).pipe(res);
}

http.createServer(function (req, res) {
  const file = resolve(req.url);
  if (file) send(res, 200, file);
  else send(res, 404, path.join(root, '404.html'));
}).listen(port, '127.0.0.1', function () {
  console.log('site on http://127.0.0.1:' + port + ' from ' + root);
});
