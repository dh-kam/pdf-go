let readyPromise = init();

self.addEventListener("message", async (event) => {
  try {
    await readyPromise;
    const message = event.data;
    postLog("worker: message received", { type: message.type });
    if (message.type === "open") {
      const startedAt = performance.now();
      postLog("worker: open start", {
        bytes: message.buffer ? message.buffer.byteLength : 0,
        backend: message.options && message.options.backend,
      });
      const result = self.pdfgo.openDocument(new Uint8Array(message.buffer), message.options || {});
      ensureOK(result);
      postLog("worker: open done", {
        id: result.id,
        pageCount: result.pageCount,
        backend: result.backend,
        elapsedMs: Math.round(performance.now() - startedAt),
      });
      self.postMessage({
        type: "opened",
        id: result.id,
        pageCount: result.pageCount,
        backend: result.backend,
        version: result.version,
      });
      return;
    }
    if (message.type === "render") {
      const startedAt = performance.now();
      postLog("worker: render start", {
        id: message.id,
        pageIndex: message.pageIndex,
        options: message.options || {},
      });
      const result = self.pdfgo.renderPage(message.id, message.pageIndex, message.options || {});
      ensureOK(result);
      postLog("worker: render done", {
        pageIndex: result.pageIndex,
        width: result.width,
        height: result.height,
        bytes: result.data ? result.data.byteLength : 0,
        elapsedMs: Math.round(performance.now() - startedAt),
      });
      self.postMessage(
        {
          type: "rendered",
          pageIndex: result.pageIndex,
          width: result.width,
          height: result.height,
          buffer: result.data.buffer,
        },
        [result.data.buffer],
      );
      return;
    }
    if (message.type === "close") {
      postLog("worker: close start", { id: message.id });
      const result = self.pdfgo.closeDocument(message.id);
      ensureOK(result);
      postLog("worker: close done", { id: message.id });
      self.postMessage({ type: "closed", id: message.id });
    }
  } catch (error) {
    postLog("worker: error", { error: error && error.message ? error.message : String(error) });
    self.postMessage({ type: "error", error: error && error.message ? error.message : String(error) });
  }
});

async function init() {
  postLog("worker: init start");
  importScripts("./wasm_exec.js");
  const go = new self.Go();
  postLog("worker: fetching wasm");
  const response = await fetch("./pdfwasm.wasm");
  const wasmBytes = await response.arrayBuffer();
  postLog("worker: compiling wasm", { bytes: wasmBytes.byteLength });
  const { instance } = await WebAssembly.instantiate(wasmBytes, go.importObject);
  go.run(instance);
  await waitForAPI();
  const version = self.pdfgo.version();
  postLog("worker: ready", { version: version && version.version });
  self.postMessage({ type: "ready", version: version && version.version });
}

async function waitForAPI() {
  for (let i = 0; i < 100; i++) {
    if (self.pdfgo) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error("pdfgo WASM API was not registered");
}

function ensureOK(result) {
  if (!result || !result.ok) {
    throw new Error((result && result.error) || "WASM call failed");
  }
}

function postLog(message, detail) {
  console.log("[pdf-go worker]", message, detail || "");
  self.postMessage({ type: "log", message, detail });
}
