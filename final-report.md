## Summary

Restored frontend dependency policy and adapter compatibility on PR #167 while preserving security upgrades that clear all 7 dependency alerts and the high audit.

### Changes Made

1. **web/pnpm-workspace.yaml** - Restored policy from main branch with security upgrades:
   - `packages: [vendor/*]` - Local vendor packages workspace
   - `autoInstallPeers: false` - Supply chain safety
   - Local link overrides for `cssom`, `html-escaper`, `uhyphen` adapters
   - `minimumReleaseAge: 10080` with exclusions for:
     - Original `@typescript-eslint/*@8.65.0` → updated to `@8.71.0`
     - `obug@2.1.4`, `brace-expansion@5.0.8`
     - New transitive deps from PR #167: `baseline-browser-mapping@2.11.27`, `browserslist@4.29.3`, `caniuse-lite@1.0.30001814`, `electron-to-chromium@1.5.444`, `postcss@8.5.29`, `source-map-js@1.2.2`
   - `patchedDependencies: minimatch@3.1.5` for brace-expansion v5 compat
   - Overrides updated to fix high vulnerabilities:
     - `"brace-expansion@<5.0.11": "5.0.12"` (was `"brace-expansion@<=5.0.7": "5.0.8"`)
     - `"postcss@<8.5.23": "8.5.29"` (was `"postcss@<=8.5.17": "8.5.19"`)

2. **web/pnpm-lock.yaml** - Regenerated with restored policy

3. **Deleted root result.md** - Accidental file removed

4. **web/package.json** - Unchanged (already had PR #167 security upgrades):
   - `@typescript-eslint/eslint-plugin`: `^8.70.1`
   - `@typescript-eslint/parser`: `^8.70.1`
   - `vitest`: `^4.1.11`

### Verification Results

| Check | Result |
|-------|--------|
| `pnpm --dir web audit --audit-level high` | ✅ No known vulnerabilities |
| `pnpm --dir web test` | ✅ 34 tests pass (including linkedom-environment & uhyphen) |
| `pnpm --dir web lint` | ✅ Pass |
| `pnpm --dir web build` | ✅ Pass |
| `bash scripts/verify-frontend-dependency-policy.sh` | ✅ Verified |

### Files Changed
- `web/pnpm-workspace.yaml` (restored policy + security upgrade exclusions)
- `web/pnpm-lock.yaml` (regenerated)
- `result.md` (deleted)
