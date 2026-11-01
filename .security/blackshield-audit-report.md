# BlackShield Security Audit Report — `relab/gorums`

- **Scanned Commit:** `3690049b98bc84f15abd7752a6d1786feecab227`
- **Actionable Findings:** `6` (`1 Critical`, `5 High`, `5 Medium`, `3 Low`)
- **BlackShield Platform & Continuous Zero-Cost PR Protection:** [https://blackshield.chaplau.com](https://blackshield.chaplau.com)

## Detailed Findings & Remediation Guide

### 1. [CRITICAL] `sec-cmdi-shell` — sec-cmdi-shell: Command Injection via Shell Invocation

- **Category:** `sast`
- **Location:** `benchkit/cmd/sweep/deploy.go:264`
- **Details:** Executing system commands through a shell interpreter with dynamic arguments permits command injection. at benchkit/cmd/sweep/deploy.go:264
- **Recommended Fix:** Invoke the target binary directly with exec.Command(bin, args...) without sh -c shell evaluation.

### 2. [HIGH] `sec-sqli-format` — sec-sqli-format: SQL Injection via Formatted String Query

- **Category:** `sast`
- **Location:** `internal/cmd/gorums-release/publish.go:159`
- **Details:** Constructing SQL queries with string formatting or concatenation can lead to SQL injection. at internal/cmd/gorums-release/publish.go:159
- **Recommended Fix:** Use parameterized queries or prepared statements instead of string concatenation.

### 3. [HIGH] `sec-sqli-format` — sec-sqli-format: SQL Injection via Formatted String Query

- **Category:** `sast`
- **Location:** `internal/cmd/gorums-release/publish.go:206`
- **Details:** Constructing SQL queries with string formatting or concatenation can lead to SQL injection. at internal/cmd/gorums-release/publish.go:206
- **Recommended Fix:** Use parameterized queries or prepared statements instead of string concatenation.

### 4. [HIGH] `sec-broken-crypto-cipher` — sec-broken-crypto-cipher: Deprecated or Weak Symmetric Cipher Algorithm

- **Category:** `sast`
- **Location:** `internal/conn/config.go:56`
- **Details:** Using broken cryptographic algorithms (DES, 3DES, RC4, Blowfish, AES-ECB mode) compromises data confidentiality. at internal/conn/config.go:56
- **Recommended Fix:** Use AES-256-GCM or ChaCha20-Poly1305 for symmetric encryption. Avoid ECB mode.

### 5. [HIGH] `sec-broken-crypto-cipher` — sec-broken-crypto-cipher: Deprecated or Weak Symmetric Cipher Algorithm

- **Category:** `sast`
- **Location:** `internal/conn/config.go:87`
- **Details:** Using broken cryptographic algorithms (DES, 3DES, RC4, Blowfish, AES-ECB mode) compromises data confidentiality. at internal/conn/config.go:87
- **Recommended Fix:** Use AES-256-GCM or ChaCha20-Poly1305 for symmetric encryption. Avoid ECB mode.

### 6. [HIGH] `sec-broken-crypto-cipher` — sec-broken-crypto-cipher: Deprecated or Weak Symmetric Cipher Algorithm

- **Category:** `sast`
- **Location:** `internal/conn/inbound_manager.go:131`
- **Details:** Using broken cryptographic algorithms (DES, 3DES, RC4, Blowfish, AES-ECB mode) compromises data confidentiality. at internal/conn/inbound_manager.go:131
- **Recommended Fix:** Use AES-256-GCM or ChaCha20-Poly1305 for symmetric encryption. Avoid ECB mode.

