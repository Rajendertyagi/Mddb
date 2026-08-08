# Windows QA Audit - README

## Overview

This audit pipeline performs comprehensive testing of the MDDB Windows executable entirely within GitHub Actions. No local builds or dependencies are required.

## What Gets Tested

### Build Verification
- Cross-compilation of `mddbd.exe` and `mddb-cli.exe`
- Embedded web UI packaging
- Binary size validation

### Functional Testing
- HTTP/JSON API endpoints
- CLI commands
- Embedded web UI accessibility
- Windows filesystem edge cases (long paths, Unicode, concurrent access)

### Security Audit
- Path traversal vulnerabilities
- Command injection risks
- Temp file security
- Authentication/authorization
- Encryption implementation

### Patch Analysis
- All 15 vendor patches reviewed
- Upstream-ability assessment
- Risk classification

## How to Run

### Manual Trigger
Go to your repository → Actions → "Windows QA & Feature Audit" → "Run workflow"

### Automatic Trigger
The workflow runs automatically on:
- Push to `main` branch (when relevant files change)
- Manual dispatch

## Output Artifacts

After completion, download these artifacts:

| Artifact | Description |
|----------|-------------|
| `mddb-windows-amd64` | Built executables (`mddbd.exe`, `mddb-cli.exe`) |
| `audit-test-results` | Test logs and database files |
| `security-audit` | Security findings and patch analysis |
| `audit-report` | Complete audit report (audit-report.md) |
| `unit-test-results` | Go test output logs |

## Interpreting Results

### Health Scores
- **90-100**: Production ready
- **70-89**: Beta ready (minor issues)
- **50-69**: Needs work (significant issues)
- **<50**: Not production ready

### Risk Levels
- **Critical**: Must fix before deployment
- **High**: Should fix soon, workarounds exist
- **Medium**: Document and monitor
- **Low**: Acceptable risk

## Known Limitations

1. **Vector Search**: Requires embedding provider API keys (not tested by default)
2. **gRPC Testing**: Requires `grpcurl` (not installed in runner)
3. **File Upload**: Requires binary upload testing (not automated)
4. **Replication**: Requires multi-node setup

## Troubleshooting

### Build Fails
- Check that all 15 patches apply cleanly
- Verify Go 1.26.5 is available in runner
- Check Node.js 24 for panel build

### Tests Fail
- Review test logs in artifacts
- Check for resource conflicts (port 11023/11024)
- Verify Windows runner has sufficient disk space

### Security Findings
- Review `security-findings.md` in artifacts
- Prioritize by severity
- Check patch analysis for root cause

## Updating the Audit

To add new tests or modify the audit:

1. Edit `.github/workflows/Mddb-Windows-Audit.yml`
2. Add test cases to the `live-functional-tests` job
3. Commit and push to trigger the workflow

## File Structure

```
.github/workflows/
  Mddb-Windows-Audit.yml    # Main audit pipeline
Mddb-patches/
  patches/                   # 15 vendor patches
  README.md                  # Patch documentation
  AUDIT-README.md           # This file
```

## Contact

For questions about the audit process, refer to:
- `Mddb-patches/WINDOWS_PORT.md` - Technical architecture
- `Mddb-patches/STATUS.md` - Current project status
- GitHub Actions logs for detailed output
