# Mock Website for Formularul 230 Autocomplete

## Overview
Implement a mock website that allows users to fill in their information for the Romanian "Formularul 230" tax form. The system will provide an interactive web form, validate the Romanian Personal Numeric Code (CNP), automatically determine the postal code based on the address, and generate a completed PDF form.

## Context
- Files involved: `cmd/f230/main.go`, `pkg/f230/cnp.go`, `pkg/f230/postal.go`, `pkg/f230/pdf.go`, `web/f230/index.html`.
- Tech Stack: Go (Backend), Vanilla HTML/JS/CSS (Frontend).
- Dependencies: `github.com/jung-kurt/gofpdf` for PDF generation.

## Development Approach
- **Testing approach**: Regular (code first, then tests).
- Complete each task fully before moving to the next.
- **CRITICAL: every task MUST include new/updated tests.**
- **CRITICAL: all tests must pass before starting next task.**

## Implementation Steps

### Task 1: CNP Validation and Postal Code Lookup

**Files:**
- Create: `pkg/f230/cnp.go`
- Create: `pkg/f230/cnp_test.go`
- Create: `pkg/f230/postal.go`
- Create: `pkg/f230/postal_test.go`

- [ ] implement Romanian CNP validation logic (13 digits, checksum calculation, birth date validation)
- [ ] implement mock postal code lookup using a static data set (e.g., city/street to code mapping)
- [ ] write comprehensive unit tests for CNP validation
- [ ] write unit tests for postal code lookup
- [ ] ensure all tests pass

### Task 2: PDF Generation Service

**Files:**
- Create: `pkg/f230/pdf.go`
- Create: `pkg/f230/pdf_test.go`

- [ ] add `github.com/jung-kurt/gofpdf` dependency to `go.mod`
- [ ] implement PDF generation logic mapping form data to a layout resembling Formularul 230
- [ ] implement a function to serve the generated PDF as a byte stream
- [ ] write tests to verify that the PDF is generated correctly with the provided data
- [ ] ensure all tests pass

### Task 3: Web Server and Frontend Development

**Files:**
- Create: `cmd/f230/main.go`
- Create: `web/f230/index.html`
- Create: `web/f230/script.js`
- Create: `web/f230/style.css`

- [ ] implement a Go HTTP server to serve the static frontend and the API
- [ ] implement API endpoint `/api/lookup-postal` to provide postal codes via AJAX
- [ ] implement API endpoint `/api/generate-pdf` to handle form submission and return the PDF
- [ ] create a responsive HTML form in `index.html` to collect user data
- [ ] implement frontend JS to handle CNP validation and postal code auto-population
- [ ] add basic styling for a professional "mock website" look

### Task 4: Integration and Verification

**Files:**
- Modify: `README.md`

- [ ] connect frontend form to the backend PDF generation API
- [ ] verify that the postal code is correctly determined when the user enters their address
- [ ] verify that invalid CNPs are caught on the frontend and backend
- [ ] add instructions to `README.md` on how to build and run the f230 tool
- [ ] run full project test suite

### Task 5: Verify acceptance criteria

- [ ] run all tests in `pkg/f230/` and `cmd/f230/`
- [ ] run linter if available
- [ ] verify that the generated PDF contains the correct user information
- [ ] verify that the postal code determination works as expected for at least 3 test cases
