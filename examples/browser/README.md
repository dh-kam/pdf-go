# Browser WebAssembly Renderer

This example renders PDF pages in a browser by loading `pdfwasm.wasm` inside a Web Worker. The worker keeps rendering off the UI thread and draws the returned RGBA pixels onto a `<canvas>`.

The demo defaults to the pure Go `splash` backend so browser rendering follows the Poppler-parity path. The `image-canvas` backend remains selectable as a diagnostic fallback.

Runtime progress is mirrored to both the page log panel and the browser console. This makes it easier to inspect worker, WASM open, and render timings through Chrome DevTools Protocol.

## Build

```bash
make build-wasm
```

The build target writes the demo bundle to:

```text
build/js-wasm/default/
```

## Run

Serve the bundle with any static HTTP server:

```bash
cd build/js-wasm/default
python3 -m http.server 8080
```

Open `http://localhost:8080`, choose a PDF file, and render a page.

The JavaScript API uses zero-based page indexes internally. The demo UI displays one-based page numbers.
