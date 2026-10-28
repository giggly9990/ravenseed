# Repository verification

The `verify` job checks Go formatting, vet and race tests, frontend formatting, tests and build, the runtime image and a Trivy scan. Action references are pinned to verified commit hashes. Dependabot checks for dependency updates weekly; [.github/dependabot.yml](../.github/dependabot.yml) lists the ecosystems it covers. Publishing requires verification to pass.

The repository settings are separate from these files. With GitHub CLI authenticated as an administrator, enable the workflow and protect `main`:

```sh
gh workflow enable ci.yml --repo giggly9990/ravenseed
gh api --method PUT repos/giggly9990/ravenseed/branches/main/protection --input .github/main-protection.json
```

The policy requires a pull request, the `verify` and `demo-smoke` checks against an up-to-date branch, resolved conversations and no force pushes or deletion. It also applies to administrators. It allows zero external approvals so the sole maintainer can merge after verification. Review existing protection before applying this file because the PUT replaces branch protection settings. Private-repository branch protection depends on the GitHub account's plan.

The `Security` workflow runs CodeQL on the Go and TypeScript code on every push, pull request and weekly, and fails pull requests that add a dependency with a known high or critical vulnerability. Findings appear under **Security > Code scanning**. Keep CodeQL's default setup off; it conflicts with this workflow. Enable the remaining security features once the repository is public:

```sh
gh api --method PUT repos/giggly9990/ravenseed/vulnerability-alerts
gh api --method PUT repos/giggly9990/ravenseed/automated-security-fixes
gh api --method PUT repos/giggly9990/ravenseed/private-vulnerability-reporting
gh api --method PATCH repos/giggly9990/ravenseed --input - <<'EOF'
{"security_and_analysis": {"secret_scanning": {"status": "enabled"}, "secret_scanning_push_protection": {"status": "enabled"}}}
EOF
```

These turn on Dependabot alerts and security fixes, private reporting through [SECURITY.md](../SECURITY.md), and secret scanning that blocks pushes containing known token formats. Dependabot security fixes target `main`, not `develop`.

Work happens on `develop`; Dependabot opens its pull requests there. Merge `develop` into `main` by pull request, then tag a release on `main` (`git tag v1.2.3 && git push origin v1.2.3`).

To verify a branch manually, use **Actions > Build and verify > Run workflow**.
