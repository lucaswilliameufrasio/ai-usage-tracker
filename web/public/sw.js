const CACHE = "shell-v2";
const PRECACHE = ["/", "/index.html", "/manifest.webmanifest", "/icon-192.png", "/icon-512.png", "/icon-maskable-512.png"];

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(CACHE).then((cache) => cache.addAll(PRECACHE)));
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil((async () => {
    const keys = await caches.keys();
    for (const key of keys) {
      if (key !== CACHE) {
        await caches.delete(key);
      }
    }
    await self.clients.claim();
  })());
});

self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);
  if (event.request.method !== "GET" || url.origin !== self.location.origin) {
    return;
  }
  if (url.pathname.startsWith("/v1/") || url.pathname === "/healthz") {
    return;
  }
  event.respondWith(networkThenCache(event.request));
});

self.addEventListener("push", (event) => {
  const payload = { title: "Uso de IA", body: "Há uma atualização de uso." };
  if (event.data) {
    try {
      const parsed = event.data.json();
      if (typeof parsed.title === "string" && parsed.title !== "") {
        payload.title = parsed.title;
      }
      if (typeof parsed.body === "string" && parsed.body !== "") {
        payload.body = parsed.body;
      }
    } catch {
      payload.body = "Há uma atualização de uso.";
    }
  }
  event.waitUntil(self.registration.showNotification(payload.title, { body: payload.body, data: { href: "/" } }));
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  event.waitUntil(self.clients.openWindow("/"));
});

async function networkThenCache(request) {
  const cache = await caches.open(CACHE);
  try {
    const response = await fetch(request);
    if (response.ok) {
      await cache.put(request, response.clone());
    }
    return response;
  } catch (error) {
    const cached = await cache.match(request);
    if (cached) {
      return cached;
    }
    if (request.mode === "navigate") {
      const shell = await cache.match("/index.html");
      if (shell) {
        return shell;
      }
    }
    throw error;
  }
}
