const http = require('node:http');
const { chromium } = require('playwright');

const port = Number(process.env.PLAYWRIGHT_DAEMON_PORT || '3000');
let browserPromise;

function getBrowser() {
  if (!browserPromise) {
    browserPromise = chromium.launch({ headless: true }).catch((err) => {
      browserPromise = undefined;
      throw err;
    });
  }
  return browserPromise;
}

async function readJSON(req) {
  const chunks = [];
  for await (const chunk of req) {
    chunks.push(chunk);
  }
  return JSON.parse(Buffer.concat(chunks).toString('utf8'));
}

async function handleFetch(req, res) {
  let payload;
  try {
    payload = await readJSON(req);
  } catch (err) {
    res.writeHead(400, { 'Content-Type': 'text/plain; charset=utf-8' });
    res.end(`invalid request body: ${err.message}`);
    return;
  }

  if (!payload || typeof payload.url !== 'string' || payload.url.trim() === '') {
    res.writeHead(400, { 'Content-Type': 'text/plain; charset=utf-8' });
    res.end('missing url');
    return;
  }

  const browser = await getBrowser();
  const context = await browser.newContext({ locale: 'en-US' });
  const page = await context.newPage();

  try {
    await page.goto(payload.url, { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForLoadState('networkidle', { timeout: 5000 }).catch(() => {});
    const content = await page.content();
    res.writeHead(200, {
      'Content-Type': 'text/html; charset=utf-8',
      'X-Final-URL': page.url(),
    });
    res.end(content);
  } catch (err) {
    res.writeHead(502, { 'Content-Type': 'text/plain; charset=utf-8' });
    res.end(err.message);
  } finally {
    await context.close();
  }
}

const server = http.createServer((req, res) => {
  if (req.method === 'GET' && req.url === '/health') {
    res.writeHead(200, { 'Content-Type': 'text/plain; charset=utf-8' });
    res.end('ok');
    return;
  }
  if (req.method === 'POST' && req.url === '/fetch') {
    handleFetch(req, res).catch((err) => {
      res.writeHead(500, { 'Content-Type': 'text/plain; charset=utf-8' });
      res.end(err.message);
    });
    return;
  }
  res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
  res.end('not found');
});

async function shutdown() {
  server.close((err) => {
    if (err) {
      console.error(`error closing server: ${err.message}`);
    }
  });
  try {
    const browser = await getBrowser();
    await browser.close();
  } catch (err) {
    console.error(`error closing browser: ${err.message}`);
  }
  process.exit(0);
}

process.on('SIGINT', () => shutdown());
process.on('SIGTERM', () => shutdown());

server.listen(port, '0.0.0.0', () => {
  process.stdout.write(`playwright fetch daemon listening on 0.0.0.0:${port}\n`);
});
