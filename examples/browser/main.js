const worker = new Worker("./pdf_worker.js");

const fileInput = document.querySelector("#file");
const pageInput = document.querySelector("#page");
const dpiInput = document.querySelector("#dpi");
const scaleInput = document.querySelector("#scale");
const backendInput = document.querySelector("#backend");
const renderButton = document.querySelector("#render");
const prevPageButton = document.querySelector("#prev-page");
const nextPageButton = document.querySelector("#next-page");
const statusNode = document.querySelector("#status");
const logNode = document.querySelector("#log");
const canvasWrap = document.querySelector("#canvas-wrap");
const canvas = document.querySelector("#canvas");
const context = canvas.getContext("2d");
const viewModeButtons = Array.from(document.querySelectorAll("button[data-view-mode]"));

let documentID = 0;
let pageCount = 0;
let busy = false;
let viewMode = "fit-width";

worker.addEventListener("message", (event) => {
  const message = event.data;
  if (message.type === "log") {
    appendLog(message.message, message.detail);
    return;
  }
  if (message.type === "ready") {
    setStatus(`WASM ready. pdf-go ${message.version || "unknown"}.`);
    updateRenderState();
    return;
  }
  if (message.type === "opened") {
    documentID = message.id;
    pageCount = message.pageCount;
    pageInput.max = String(pageCount);
    pageInput.value = "1";
    setStatus(`Opened ${pageCount} page(s) with ${message.backend}.`);
    busy = false;
    updateRenderState();
    renderCurrentPage();
    return;
  }
  if (message.type === "rendered") {
    const pixels = new Uint8ClampedArray(message.buffer);
    canvas.width = message.width;
    canvas.height = message.height;
    canvasWrap.dataset.rendered = "true";
    context.putImageData(new ImageData(pixels, message.width, message.height), 0, 0);
    setStatus(`Rendered page ${message.pageIndex + 1}/${pageCount} at ${message.width}x${message.height}.`);
    busy = false;
    updateRenderState();
    return;
  }
  if (message.type === "error") {
    setStatus(message.error || "Unknown WASM renderer error.");
    busy = false;
    updateRenderState();
  }
});

fileInput.addEventListener("change", async () => {
  const file = fileInput.files && fileInput.files[0];
  if (!file) {
    return;
  }
  if (busy) {
    appendLog("main: file change ignored while busy");
    return;
  }
  busy = true;
  documentID = 0;
  pageCount = 0;
  canvasWrap.dataset.rendered = "false";
  updateRenderState();
  setStatus(`Loading ${file.name}...`);
  appendLog("main: file selected", { file: file.name, size: file.size, backend: backendInput.value });
  let buffer;
  try {
    appendLog("main: file arrayBuffer start", { file: file.name });
    buffer = await file.arrayBuffer();
    appendLog("main: file arrayBuffer done", { bytes: buffer.byteLength });
  } catch (error) {
    setStatus(error && error.message ? error.message : String(error));
    busy = false;
    updateRenderState();
    return;
  }
  appendLog("main: open requested", {
    file: file.name,
    size: file.size,
    backend: backendInput.value,
  });
  worker.postMessage(
    {
      type: "open",
      buffer,
      options: {
        backend: backendInput.value,
        maxWorkers: 1,
        cacheSize: 4,
      },
    },
    [buffer],
  );
});

renderButton.addEventListener("click", () => {
  renderCurrentPage();
});

prevPageButton.addEventListener("click", () => {
  renderAdjacentPage(-1);
});

nextPageButton.addEventListener("click", () => {
  renderAdjacentPage(1);
});

for (const button of viewModeButtons) {
  button.addEventListener("click", () => {
    setViewMode(button.dataset.viewMode);
  });
}

pageInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter") {
    renderCurrentPage();
  }
});

pageInput.addEventListener("input", () => {
  updateRenderState();
});

function renderAdjacentPage(delta) {
  if (!documentID || busy) {
    return;
  }
  const currentPage = clampNumber(Number(pageInput.value || 1), 1, pageCount || 1);
  const nextPage = clampNumber(currentPage + delta, 1, pageCount || 1);
  if (nextPage === currentPage) {
    return;
  }
  pageInput.value = String(nextPage);
  renderCurrentPage();
}

function renderCurrentPage() {
  if (!documentID || busy) {
    return;
  }
  const pageNumber = clampNumber(Number(pageInput.value || 1), 1, pageCount || 1);
  pageInput.value = String(pageNumber);
  busy = true;
  updateRenderState();
  setStatus(`Rendering page ${pageNumber}...`);
  appendLog("main: render requested", {
    documentID,
    pageIndex: pageNumber - 1,
    dpi: Number(dpiInput.value || 144),
    scale: Number(scaleInput.value || 1),
  });
  worker.postMessage({
    type: "render",
    id: documentID,
    pageIndex: pageNumber - 1,
    options: {
      dpi: Number(dpiInput.value || 144),
      scale: Number(scaleInput.value || 1),
      timeoutMs: 0,
      maxPagePixels: 80000000,
      enableCache: true,
    },
  });
}

function updateRenderState() {
  const currentPage = clampNumber(Number(pageInput.value || 1), 1, pageCount || 1);
  renderButton.disabled = busy || !documentID;
  prevPageButton.disabled = busy || !documentID || currentPage <= 1;
  nextPageButton.disabled = busy || !documentID || currentPage >= pageCount;
}

function setViewMode(nextMode) {
  if (nextMode !== "actual-size" && nextMode !== "fit-width") {
    return;
  }
  viewMode = nextMode;
  canvasWrap.dataset.viewMode = viewMode;
  for (const button of viewModeButtons) {
    button.setAttribute("aria-pressed", String(button.dataset.viewMode === viewMode));
  }
  appendLog("main: view mode changed", { viewMode });
}

function setStatus(text) {
  statusNode.textContent = text;
  appendLog(`status: ${text}`);
}

function clampNumber(value, min, max) {
  if (!Number.isFinite(value)) {
    return min;
  }
  return Math.max(min, Math.min(max, Math.trunc(value)));
}

function appendLog(message, detail) {
  const timestamp = new Date().toISOString();
  const suffix = detail === undefined ? "" : ` ${JSON.stringify(detail)}`;
  const line = `[${timestamp}] ${message}${suffix}`;
  console.log("[pdf-go]", message, detail || "");
  logNode.textContent += `${line}\n`;
  logNode.scrollTop = logNode.scrollHeight;
}
