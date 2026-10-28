# Repository verification

The `verify` job checks Go formatting, vet and race tests, frontend formatting, tests and build, the runtime image and a Trivy scan. Action references are pinned to verified commit hashes. Dependabot checks for dependency updates weekly; [.github/dependabot.yml](../.github/dependabot.yml) lists the ecosystems it covers. Publishing requires verification to pass.

The repository settings are separate from these files. With GitHub CLI authenticated as an administrator, enable the workflow and protect `main`:

```sh
gh workflow enable ci.yml --repo giggly9990/ravenseed
gh api --method PUT repos/giggly9990/ravenseed/branches/main/protection --input .github/main-protection.json
```

The policy requires a pull request, the `verify` check against an up-to-date branch, resolved conversations and no force pushes or deletion. It also applies to administrators. It allows zero external approvals so the sole maintainer can merge after verification. Review existing protection before applying this file because the PUT replaces branch protection settings. Private-repository branch protection depends on the GitHub account's plan.

To verify a branch manually, use **Actions > Build and verify > Run workflow**.
