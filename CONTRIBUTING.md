# Contributing to LITEPLOY

Thank you for your interest in contributing to LITEPLOY! We welcome pull requests, bug reports, feature suggestions, and documentation improvements.

---

## 🛠️ Local Development Setup

### Prerequisites
- **Go:** 1.23+ (Go 1.26 recommended)
- **Docker:** Docker Engine 20.10+ running locally or via Docker Desktop.
- **Git:** Git 2.30+

### Step-by-Step Instructions

1. **Clone the Repository:**
   ```bash
   git clone https://github.com/nuexn0x-9/liteploy.git
   cd liteploy
   ```

2. **Run Tests:**
   ```bash
   go test -v ./...
   ```

3. **Build the Binary:**
   ```bash
   make build
   # or: go build -o bin/liteploy ./cmd/liteploy
   ```

4. **Run LITEPLOY in Development Mode:**
   ```bash
   LITEPLOY_DEV_MODE=true ./bin/liteploy
   ```
   Open `http://localhost:8080` in your browser.

---

## 🧪 Testing Guidelines

- Always run tests before submitting a PR:
  ```bash
  go fmt ./...
  go vet ./...
  go test -v ./...
  ```
- New features should include unit tests in the relevant `internal/<package>/` directory.
- Ensure all existing end-to-end tests (`tests/e2e_test.go`) pass cleanly.

---

## 📋 Pull Request Expectations

1. **Keep it Lightweight:** LITEPLOY prioritizes small RAM usage and simple architecture. PRs introducing heavy external infrastructure (databases, caches, heavy JS frameworks) will be rejected unless explicitly planned.
2. **Follow Existing Style:** Keep code clean, idiomatic, and documented using Go conventions and `log/slog`.
3. **Descriptive Commits:** Write clear commit messages explaining *what* changed and *why*.
