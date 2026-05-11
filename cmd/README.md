# Go PDF CLI Tools

This directory contains three command-line tools for working with PDF documents:

**Version: 0.9.0-202602.1**

Use `-v` or `--version` on any tool to print the current version.

## Tools

### pdfinfo
Display PDF document information including metadata, page count, dimensions, and more.

**Usage:**
```bash
pdfinfo [options] <pdf-file> [pdf-file ...]
```

**Options:**
- `-h, --help` - Show help message and exit
- `-v, --version` - Show version information and exit
- `-j, --json` - Output information in JSON format
- `-v, --verbose` - Enable verbose output
- `-p, --page-details` - Show detailed information about each page
- `-m, --metadata` - Show XMP metadata
- `-o, --outlines` - Show document outlines/bookmarks
- `-f, --form-fields` - Show flattened AcroForm fields

**Examples:**
```bash
pdfinfo document.pdf
pdfinfo -j document.pdf
pdfinfo -p document.pdf
pdfinfo -p -m document.pdf
pdfinfo -o document.pdf
pdfinfo -f document.pdf
pdfinfo *.pdf
```

### pdftext
Extract text content from PDF documents.

**Usage:**
```bash
pdftext [options] <pdf-file> [pdf-file ...]
```

**Options:**
- `-h, --help` - Show help message and exit
- `-v, --version` - Show version information and exit
- `-j, --json` - Output text in JSON format
- `-o, --output FILE` - Write output to file instead of stdout
- `-p, --pages SPEC` - Extract specific pages (e.g., "1", "1-5", "1,3,5")
- `-l, --layout` - Preserve layout (try to maintain original formatting)
- `--positions` - Include text position information

**Page Specification:**
- Single page: `1`
- Range: `1-5`
- Comma-separated: `1,3,5`
- Combined: `1-3,5,7-9`

**Examples:**
```bash
pdftext document.pdf
pdftext -o output.txt document.pdf
pdftext -p 1-3 document.pdf
pdftext -j document.pdf
pdftext --positions document.pdf
pdftext -p 1,3,5-7 document.pdf
```

### pdfrender
Render PDF pages to images.

**Usage:**
```bash
pdfrender [options] <pdf-file> [pdf-file ...]
```

**Options:**
- `-h, --help` - Show help message and exit
- `-v, --version` - Show version information and exit
- `-o, --output DIR` - Output directory (default: <filename>_rendered)
- `-p, --pages SPEC` - Render specific pages (e.g., "1", "1-5", "1,3,5")
- `-d, --dpi DPI` - Render at specified DPI (default: 72)
- `-s, --scale SCALE` - Scale factor (default: 1.0)
- `-f, --format FMT` - Output format (png, jpg) (default: png)
- `--prefix PREFIX` - Prefix for output files (default: input filename)
- `-w, --workers N` - Number of concurrent render workers (default: 4)
- `--cache-size N` - Rendered-page cache size when cache is enabled (0: auto by workers, default auto=8 for 4 workers)
- `--cache-ttl-sec N` - Rendered-page cache TTL in seconds when cache is enabled (default: 300)
- `--max-page-pixels N` - Per-page pixel limit (skip/fail page when exceeded, 0: unlimited)
- `--max-inflight-pixels N` - Worker auto-throttle based on worst-page pixel budget (0: unlimited)
- `-q, --quiet` - Suppress progress output
- `--enable-cache` - Enable rendered-page cache during this command (default: disabled)
- `--password PASSWORD` - Password for encrypted PDF files
- `--fail-on-page-error` - Exit with non-zero status when any page render fails

**Examples:**
```bash
pdfrender document.pdf
pdfrender -o output document.pdf
pdfrender -p 1-3 document.pdf
pdfrender -d 150 document.pdf
pdfrender -s 2.0 document.pdf
pdfrender -p 1,3,5-7 document.pdf
pdfrender -w 8 document.pdf
pdfrender --password openpassword protected.pdf
pdfrender --fail-on-page-error -d 150 document.pdf
pdfrender --enable-cache -d 300 document.pdf
pdfrender --enable-cache --cache-size 12 --cache-ttl-sec 600 document.pdf
pdfrender --max-page-pixels 4000000 --max-inflight-pixels 8000000 -d 300 large.pdf
```

## Building

Build all tools:
```bash
make build-all
```

Build individual tools:
```bash
make build-pdfinfo
make build-pdftext
make build-pdfrender
```

The binaries will be created in the `bin/` directory.

## Architecture

These tools follow Clean Architecture principles:

- **cmd/** - Entry points for CLI applications
- **internal/domain/** - Core business logic and entities
- **internal/infrastructure/** - External dependencies (PDF parsing, rendering, text extraction)
- **internal/usecase/** - Application use cases

## Dependencies

The tools use the following infrastructure components:

- **Parser** (`internal/infrastructure/pdf/xref`) - PDF cross-reference table parsing
- **Renderer** (`internal/infrastructure/renderer`) - Concurrent page rendering
- **Text Extractor** (`internal/infrastructure/text`) - Text extraction from PDF content streams
- **Entity Layer** (`internal/domain/entity`) - PDF document and page entities
