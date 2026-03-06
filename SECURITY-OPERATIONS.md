# Security Operations Memo

This repository uses GitHub-native controls to reduce supply chain and secret leakage risk.

## Current Controls

- `CI` runs `go vet`, `go test -race`, and `govulncheck`.
- `CodeQL` scans Go code on pull requests, pushes to `main`, and weekly.
- `Dependency Review` blocks pull requests that introduce vulnerable dependencies.
- `SBOM` generates a CycloneDX dependency inventory on `main`, on demand, and weekly.
- `security-settings-audit` checks that required GitHub security features stay enabled.
- Dependabot updates Go modules and GitHub Actions weekly.

## GitHub Settings That Must Stay Enabled

Open `Settings -> Advanced Security` for `kubot64/conflux` and keep these enabled:

- `Dependency graph`
- `Dependabot alerts`
- `Dependabot security updates`
- `Secret scanning`
- `Push protection`
- `Private vulnerability reporting`

These settings are required for the workflows in `.github/workflows/` to work as intended.

## Branch Protection Expectations

Open `Settings -> Rules -> Rulesets` and keep the `main` branch protected with:

- pull request required before merge
- at least 1 approval
- required status checks enabled
- force push blocked
- branch deletion blocked

Recommended required checks:

- `CI / Test`
- `CI / Vulnerability scan`
- `CodeQL / Analyze (Go)`
- `Dependency Review / Dependency Review`

## Operating Notes

- All GitHub Actions must be pinned to a full commit SHA. Tag-only references such as `@v4` will be rejected by repository rules.
- `Auto-merge minor/patch updates` is expected to show `skipped` on non-Dependabot pull requests.
- `Dependency Review` requires `Dependency graph` to be enabled in the repository settings.
- The SBOM artifact is uploaded as `sbom-cyclonedx` from the `SBOM` workflow.

## If A Check Fails

### Dependency Review

Common causes:

- `Dependency graph` is disabled
- a dependency with a known vulnerability was added or updated
- the workflow references an action without a full SHA pin

First checks:

1. Open `Settings -> Advanced Security` and confirm `Dependency graph` is enabled.
2. Open the failed workflow log and check whether the failure is configuration-related or dependency-related.
3. If settings were just changed, rerun the failed job from the pull request.

### Security Settings Audit

This workflow usually fails because a required GitHub security feature was disabled in the UI.

Check:

1. `Settings -> Advanced Security`
2. `Settings -> Rules -> Rulesets`
3. Re-run the workflow after restoring the setting

## Change History

Introduced on 2026-03-06:

- dependency review workflow
- CycloneDX SBOM generation workflow
- extended security settings audit for Dependabot coverage
- least-privilege adjustment for Dependabot auto-merge workflow
